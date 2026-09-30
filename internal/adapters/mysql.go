package adapters

import (
	"context"
	"database/sql"
	"github.com/seedflux/seedflux/internal/schema"
	"strings"
)

type mysqlAdapter struct{ db *sql.DB }

func OpenMySQL(dsn string) (Adapter, error) {
	dsn = strings.TrimPrefix(dsn, "mysql://")
	db, e := sql.Open("mysql", dsn)
	if e != nil {
		return nil, e
	}
	return &mysqlAdapter{db}, nil
}
func (a *mysqlAdapter) Dialect() string                          { return "mysql" }
func (a *mysqlAdapter) DB() *sql.DB                              { return a.db }
func (a *mysqlAdapter) Quote(s string) string                    { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
func (a *mysqlAdapter) Begin(c context.Context) (*sql.Tx, error) { return a.db.BeginTx(c, nil) }
func (a *mysqlAdapter) Inspect(ctx context.Context) (*schema.Schema, error) {
	s := &schema.Schema{Dialect: a.Dialect()}
	r, e := a.db.QueryContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name`)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	for r.Next() {
		var n string
		r.Scan(&n)
		t := &schema.Table{Name: n}
		c, e := a.db.QueryContext(ctx, `SELECT column_name,column_type,is_nullable,COALESCE(column_default,''),extra FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ordinal_position`, n)
		if e != nil {
			return nil, e
		}
		for c.Next() {
			var name, typ, nullable, def, extra string
			c.Scan(&name, &typ, &nullable, &def, &extra)
			t.Columns = append(t.Columns, &schema.Column{Name: name, Type: typ, Nullable: nullable == "YES", Default: def, Generated: strings.Contains(extra, "GENERATED"), Identity: strings.Contains(extra, "auto_increment")})
		}
		c.Close()
		if e = mysqlConstraints(ctx, a.db, t, n); e != nil {
			return nil, e
		}
		s.Tables = append(s.Tables, t)
	}
	return s, r.Err()
}
func mysqlConstraints(ctx context.Context, db *sql.DB, t *schema.Table, n string) error {
	r, e := db.QueryContext(ctx, `SELECT constraint_name,constraint_type FROM information_schema.table_constraints WHERE table_schema=DATABASE() AND table_name=?`, n)
	if e != nil {
		return e
	}
	defer r.Close()
	for r.Next() {
		var name, kind string
		r.Scan(&name, &kind)
		cr, e := db.QueryContext(ctx, `SELECT column_name,referenced_table_name,referenced_column_name FROM information_schema.key_column_usage WHERE table_schema=DATABASE() AND table_name=? AND constraint_name=? ORDER BY ordinal_position`, n, name)
		if e != nil {
			return e
		}
		var cols []string
		f := &schema.ForeignKey{Name: name}
		for cr.Next() {
			var c string
			var rt, rc sql.NullString
			cr.Scan(&c, &rt, &rc)
			cols = append(cols, c)
			if rt.Valid {
				f.RefTable = rt.String
				f.Columns = append(f.Columns, c)
				f.RefColumns = append(f.RefColumns, rc.String)
			}
		}
		cr.Close()
		switch kind {
		case "PRIMARY KEY":
			t.PrimaryKey = cols
		case "UNIQUE":
			t.Uniques = append(t.Uniques, cols)
		case "FOREIGN KEY":
			t.ForeignKeys = append(t.ForeignKeys, f)
		}
	}
	return r.Err()
}
