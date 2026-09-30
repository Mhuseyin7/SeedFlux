package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/seedflux/seedflux/internal/schema"
	"strings"
)

type sqliteAdapter struct{ db *sql.DB }

func OpenSQLite(dsn string) (Adapter, error) {
	dsn = strings.TrimPrefix(dsn, "sqlite://")
	if dsn == "" {
		dsn = ":memory:"
	}
	db, e := sql.Open("sqlite", dsn)
	if e != nil {
		return nil, e
	}
	if _, e = db.Exec("PRAGMA foreign_keys = ON"); e != nil {
		db.Close()
		return nil, e
	}
	return &sqliteAdapter{db}, nil
}
func (a *sqliteAdapter) Dialect() string                          { return "sqlite" }
func (a *sqliteAdapter) DB() *sql.DB                              { return a.db }
func (a *sqliteAdapter) Quote(s string) string                    { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func (a *sqliteAdapter) Begin(c context.Context) (*sql.Tx, error) { return a.db.BeginTx(c, nil) }
func (a *sqliteAdapter) Inspect(ctx context.Context) (*schema.Schema, error) {
	s := &schema.Schema{Dialect: a.Dialect()}
	rows, e := a.db.QueryContext(ctx, `SELECT name, sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var name, ddl string
		if e = rows.Scan(&name, &ddl); e != nil {
			return nil, e
		}
		t := &schema.Table{Name: name}
		cr, e := a.db.QueryContext(ctx, `PRAGMA table_xinfo(`+a.Quote(name)+`)`)
		if e != nil {
			return nil, e
		}
		for cr.Next() {
			var cid, pk, hidden int
			var cn, typ string
			var notnull int
			var def sql.NullString
			if e = cr.Scan(&cid, &cn, &typ, &notnull, &def, &pk, &hidden); e != nil {
				cr.Close()
				return nil, e
			}
			c := &schema.Column{Name: cn, Type: typ, Nullable: notnull == 0, Generated: hidden > 0, Identity: pk > 0 && strings.Contains(strings.ToUpper(typ), "INT")}
			if def.Valid {
				c.Default = def.String
			}
			t.Columns = append(t.Columns, c)
			if pk > 0 {
				t.PrimaryKey = append(t.PrimaryKey, cn)
			}
		}
		cr.Close()
		fr, e := a.db.QueryContext(ctx, `PRAGMA foreign_key_list(`+a.Quote(name)+`)`)
		if e != nil {
			return nil, e
		}
		fm := map[int]*schema.ForeignKey{}
		for fr.Next() {
			var id, seq int
			var rt, from, to, onup, ondel, match string
			if e = fr.Scan(&id, &seq, &rt, &from, &to, &onup, &ondel, &match); e != nil {
				fr.Close()
				return nil, e
			}
			f := fm[id]
			if f == nil {
				f = &schema.ForeignKey{Name: fmt.Sprintf("fk_%s_%d", name, id), RefTable: rt}
				fm[id] = f
				t.ForeignKeys = append(t.ForeignKeys, f)
			}
			f.Columns = append(f.Columns, from)
			f.RefColumns = append(f.RefColumns, to)
		}
		fr.Close()
		t.Checks = checks(ddl) // SQLite exposes unique indexes reliably.
		applyCheckEnums(t)
		ir, e := a.db.QueryContext(ctx, `PRAGMA index_list(`+a.Quote(name)+`)`)
		if e != nil {
			return nil, e
		}
		for ir.Next() {
			var seq int
			var idx, origin string
			var unique, partial int
			if e = ir.Scan(&seq, &idx, &unique, &origin, &partial); e != nil {
				ir.Close()
				return nil, e
			}
			if unique == 0 || origin == "pk" {
				continue
			}
			xr, e := a.db.QueryContext(ctx, `PRAGMA index_info(`+a.Quote(idx)+`)`)
			if e != nil {
				ir.Close()
				return nil, e
			}
			var u []string
			for xr.Next() {
				var n, cid int
				var cn string
				xr.Scan(&n, &cid, &cn)
				u = append(u, cn)
			}
			xr.Close()
			if len(u) > 0 {
				t.Uniques = append(t.Uniques, u)
			}
		}
		ir.Close()
		s.Tables = append(s.Tables, t)
	}
	return s, rows.Err()
}
func checks(ddl string) []string {
	up := strings.ToUpper(ddl)
	var out []string
	for at := 0; ; {
		i := strings.Index(up[at:], "CHECK")
		if i < 0 {
			break
		}
		i += at
		open := strings.Index(ddl[i:], "(")
		if open < 0 {
			break
		}
		open += i
		dep := 0
		for j := open; j < len(ddl); j++ {
			if ddl[j] == '(' {
				dep++
			}
			if ddl[j] == ')' {
				dep--
				if dep == 0 {
					out = append(out, strings.TrimSpace(ddl[open+1:j]))
					at = j + 1
					break
				}
			}
		}
		if at <= i {
			break
		}
	}
	return out
}

// applyCheckEnums extracts the common `column IN ('a','b')` form. Other checks
// remain recorded on the normalized model and are enforced by the database.
func applyCheckEnums(t *schema.Table) {
	for _, check := range t.Checks {
		upper := strings.ToUpper(check)
		at := strings.Index(upper, " IN (")
		if at < 1 || !strings.HasSuffix(strings.TrimSpace(check), ")") {
			continue
		}
		name := strings.Trim(strings.TrimSpace(check[:at]), `"`)
		col := t.Column(name)
		if col == nil {
			continue
		}
		inside := strings.TrimSpace(check[at+5 : len(strings.TrimSpace(check))-1])
		for _, value := range strings.Split(inside, ",") {
			value = strings.Trim(strings.TrimSpace(value), "'\"")
			if value != "" {
				col.Enum = append(col.Enum, value)
			}
		}
	}
}
