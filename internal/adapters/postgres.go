package adapters

import (
	"context"
	"database/sql"
	"github.com/seedflux/seedflux/internal/schema"
	"strings"
)

type pgAdapter struct{ db *sql.DB }

func OpenPostgres(dsn string) (Adapter, error) {
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		return nil, e
	}
	return &pgAdapter{db}, nil
}
func (a *pgAdapter) Dialect() string                          { return "postgres" }
func (a *pgAdapter) DB() *sql.DB                              { return a.db }
func (a *pgAdapter) Quote(s string) string                    { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func (a *pgAdapter) Begin(c context.Context) (*sql.Tx, error) { return a.db.BeginTx(c, nil) }
func (a *pgAdapter) Inspect(ctx context.Context) (*schema.Schema, error) {
	s := &schema.Schema{Dialect: a.Dialect()}
	q := `SELECT table_schema,table_name FROM information_schema.tables WHERE table_type='BASE TABLE' AND table_schema NOT IN ('pg_catalog','information_schema') ORDER BY 1,2`
	r, e := a.db.QueryContext(ctx, q)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	for r.Next() {
		var ns, n string
		r.Scan(&ns, &n)
		t := &schema.Table{Schema: ns, Name: n}
		c, e := a.db.QueryContext(ctx, `SELECT column_name,data_type,is_nullable,column_default,is_generated FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 ORDER BY ordinal_position`, ns, n)
		if e != nil {
			return nil, e
		}
		for c.Next() {
			var name, typ, nullable, def, gen string
			c.Scan(&name, &typ, &nullable, &def, &gen)
			t.Columns = append(t.Columns, &schema.Column{Name: name, Type: typ, Nullable: nullable == "YES", Default: def, Generated: gen == "ALWAYS", Identity: strings.Contains(def, "nextval(")})
		}
		c.Close()
		if e = pgConstraints(ctx, a.db, t, ns, n); e != nil {
			return nil, e
		}
		s.Tables = append(s.Tables, t)
	}
	return s, r.Err()
}
func pgConstraints(ctx context.Context, db *sql.DB, t *schema.Table, ns, n string) error {
	r, e := db.QueryContext(ctx, `SELECT tc.constraint_name,tc.constraint_type,kcu.column_name,ccu.table_name,ccu.column_name,pgc.condeferrable FROM information_schema.table_constraints tc LEFT JOIN information_schema.key_column_usage kcu ON tc.constraint_name=kcu.constraint_name AND tc.table_schema=kcu.table_schema LEFT JOIN information_schema.constraint_column_usage ccu ON tc.constraint_name=ccu.constraint_name AND tc.table_schema=ccu.constraint_schema LEFT JOIN pg_constraint pgc ON pgc.conname=tc.constraint_name WHERE tc.table_schema=$1 AND tc.table_name=$2 ORDER BY tc.constraint_name,kcu.ordinal_position`, ns, n)
	if e != nil {
		return e
	}
	defer r.Close()
	fks := map[string]*schema.ForeignKey{}
	us := map[string][]string{}
	for r.Next() {
		var name, kind, col string
		var rt, rc sql.NullString
		var deferable sql.NullBool
		if e = r.Scan(&name, &kind, &col, &rt, &rc, &deferable); e != nil {
			return e
		}
		switch kind {
		case "PRIMARY KEY":
			t.PrimaryKey = append(t.PrimaryKey, col)
		case "UNIQUE":
			us[name] = append(us[name], col)
		case "FOREIGN KEY":
			f := fks[name]
			if f == nil {
				f = &schema.ForeignKey{Name: name, RefTable: rt.String, Deferrable: deferable.Bool}
				fks[name] = f
				t.ForeignKeys = append(t.ForeignKeys, f)
			}
			f.Columns = append(f.Columns, col)
			f.RefColumns = append(f.RefColumns, rc.String)
		}
	}
	for _, u := range us {
		t.Uniques = append(t.Uniques, u)
	}
	return r.Err()
}
