package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/seedflux/seedflux/internal/adapters"
	"github.com/seedflux/seedflux/internal/config"
	"github.com/seedflux/seedflux/internal/generator"
	"github.com/seedflux/seedflux/internal/graph"
	"github.com/seedflux/seedflux/internal/schema"
	"github.com/seedflux/seedflux/internal/writer"
	"math/rand/v2"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	RunID       string                `json:"run_id"`
	Counts      map[string]int        `json:"counts"`
	Fingerprint string                `json:"schema_fingerprint"`
	Plan        graph.Plan            `json:"-"`
	Samples     map[string]writer.Row `json:"samples,omitempty"`
}

func Plan(s *schema.Schema) (graph.Plan, error) { return graph.Build(s) }
func Generate(ctx context.Context, a adapters.Adapter, s *schema.Schema, c *config.Config, dry bool) (Result, error) {
	p, e := Plan(s)
	if e != nil {
		return Result{}, e
	}
	r := Result{RunID: fmt.Sprintf("sf_%x", uint64(time.Now().UnixNano())), Counts: map[string]int{}, Fingerprint: s.Fingerprint(), Plan: p, Samples: map[string]writer.Row{}}
	counts := countsFor(s, c, p, c.Seed)
	for k, v := range counts {
		r.Counts[k] = v
	}
	if dry {
		return r, nil
	}
	tx, e := a.Begin(ctx)
	if e != nil {
		return r, e
	}
	defer tx.Rollback()
	if e = ensureManifest(ctx, tx, a); e != nil {
		return r, e
	}
	pools := map[string][]writer.Row{}
	vs := generator.New(c.Seed)
	for _, name := range p.Order {
		t := s.Table(name)
		if t == nil {
			continue
		}
		rows, e := makeRows(t, counts[name], s, c, vs, pools)
		if e != nil {
			return r, e
		}
		if len(rows) > 0 {
			r.Samples[name] = rows[0]
		}
		if e = writer.Insert(ctx, tx, a, t, rows); e != nil {
			return r, e
		}
		if e = captureKeys(ctx, tx, a, r.RunID, t, rows); e != nil {
			return r, e
		}
		pools[name] = rows
	}
	manifest, _ := json.Marshal(r.Counts)
	if _, e = tx.ExecContext(ctx, manifestSQL(a), r.RunID, time.Now().UTC().Format(time.RFC3339), r.Fingerprint, c.Seed, string(manifest)); e != nil {
		return r, e
	}
	if e = tx.Commit(); e != nil {
		return r, e
	}
	return r, nil
}

// Rows builds the exact deterministic, FK-consistent dataset used by generate.
// It is shared by dry-run and export and intentionally performs no database writes.
func Rows(s *schema.Schema, c *config.Config) (graph.Plan, map[string][]writer.Row, map[string]int, error) {
	p, err := Plan(s)
	if err != nil {
		return graph.Plan{}, nil, nil, err
	}
	counts := countsFor(s, c, p, c.Seed)
	pools := map[string][]writer.Row{}
	vs := generator.New(c.Seed)
	for _, name := range p.Order {
		t := s.Table(name)
		if t == nil {
			continue
		}
		rows, err := makeRows(t, counts[name], s, c, vs, pools)
		if err != nil {
			return p, nil, nil, err
		}
		pools[name] = rows
	}
	return p, pools, counts, nil
}
func countsFor(s *schema.Schema, c *config.Config, p graph.Plan, seed int64) map[string]int {
	out := map[string]int{}
	rr := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b9))
	for _, n := range p.Order {
		tc := c.Tables[n]
		if tc.Count.PerParent != nil {
			parents := out[tc.Count.PerParent.Table]
			for i := 0; i < parents; i++ {
				out[n] += tc.Count.PerParent.Min + rr.IntN(tc.Count.PerParent.Max-tc.Count.PerParent.Min+1)
			}
		} else if tc.Count.Value > 0 {
			out[n] = tc.Count.Value
		} else {
			out[n] = 10
		}
	}
	return out
}
func makeRows(t *schema.Table, n int, s *schema.Schema, c *config.Config, vs generator.ValueSource, pools map[string][]writer.Row) ([]writer.Row, error) {
	out := make([]writer.Row, 0, n)
	for i := 0; i < n; i++ {
		r := writer.Row{}
		for _, col := range t.Columns {
			if col.Generated || col.Identity || col.Default != "" {
				continue
			}
			var fk *schema.ForeignKey
			var pos int
			for _, f := range t.ForeignKeys {
				for j, x := range f.Columns {
					if x == col.Name {
						fk = f
						pos = j
						break
					}
				}
			}
			if fk != nil {
				// A self-reference can safely point to the row itself when its key
				// is client-generated. This avoids disabling constraints globally.
				if fk.RefTable == t.Name {
					if key, ok := r[fk.RefColumns[pos]]; ok {
						r[col.Name] = key
						continue
					}
				}
				parents := pools[fk.RefTable]
				if len(parents) == 0 {
					if col.Nullable {
						r[col.Name] = nil
						continue
					}
					return nil, fmt.Errorf("cannot populate %s.%s: referenced table %s has no generated key pool", t.Name, col.Name, fk.RefTable)
				}
				r[col.Name] = parents[i%len(parents)][fk.RefColumns[pos]]
				continue
			}
			cc := c.Tables[t.Name].Columns[col.Name]
			v, e := vs.Value(t, col, i, cc)
			if e != nil {
				return nil, e
			}
			r[col.Name] = v
		}
		applyChecks(t, r)
		out = append(out, r)
	}
	return out, nil
}

var (
	betweenCheck = regexp.MustCompile(`(?i)^\s*["` + "`" + `]?([a-z_][a-z0-9_]*)["` + "`" + `]?\s+BETWEEN\s+(-?[0-9.]+)\s+AND\s+(-?[0-9.]+)\s*$`)
	boundCheck   = regexp.MustCompile(`(?i)^\s*["` + "`" + `]?([a-z_][a-z0-9_]*)["` + "`" + `]?\s*(>=|>)\s*(-?[0-9.]+)\s*$`)
	lengthCheck  = regexp.MustCompile(`(?i)^\s*length\s*\(\s*["` + "`" + `]?([a-z_][a-z0-9_]*)["` + "`" + `]?\s*\)\s*=\s*([0-9]+)\s*$`)
)

// applyChecks supports the deterministic subset advertised by SeedFlux. More
// complex checks remain database-enforced and should be paired with a config override.
func applyChecks(t *schema.Table, row writer.Row) {
	for _, check := range t.Checks {
		if m := betweenCheck.FindStringSubmatch(check); len(m) > 0 {
			setMinimum(row, m[1], m[2])
			continue
		}
		if m := boundCheck.FindStringSubmatch(check); len(m) > 0 {
			setMinimum(row, m[1], m[3])
			continue
		}
		if m := lengthCheck.FindStringSubmatch(check); len(m) > 0 {
			n, _ := strconv.Atoi(m[2])
			if v, ok := row[m[1]]; ok && v != nil {
				s := fmt.Sprint(v)
				if len(s) > n {
					s = s[:n]
				}
				for len(s) < n {
					s += "x"
				}
				row[m[1]] = s
			}
		}
	}
}
func setMinimum(row writer.Row, column, raw string) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return
	}
	candidate := v + 0.01
	if existing, ok := row[column]; ok {
		if f, e := strconv.ParseFloat(fmt.Sprint(existing), 64); e == nil && f >= candidate {
			return
		}
	}
	row[column] = candidate
}
func ensureManifest(ctx context.Context, tx *sql.Tx, a adapters.Adapter) error {
	q := `CREATE TABLE IF NOT EXISTS _seedflux_runs (run_id VARCHAR(64) PRIMARY KEY, created_at VARCHAR(40) NOT NULL, schema_fingerprint VARCHAR(128) NOT NULL, seed BIGINT NOT NULL, counts TEXT NOT NULL)`
	if _, e := tx.ExecContext(ctx, q); e != nil {
		return e
	}
	_, e := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _seedflux_run_rows (run_id VARCHAR(64) NOT NULL, table_name VARCHAR(255) NOT NULL, key_json TEXT NOT NULL)`)
	return e
}
func captureKeys(ctx context.Context, tx *sql.Tx, a adapters.Adapter, run string, t *schema.Table, rows []writer.Row) error {
	if len(t.PrimaryKey) == 0 {
		return nil
	}
	q := manifestRowsSQL(a)
	for _, row := range rows {
		key := map[string]any{}
		for _, col := range t.PrimaryKey {
			v, ok := row[col]
			if !ok {
				return fmt.Errorf("cannot safely record %s: database-generated primary key %s is not yet captured", t.Name, col)
			}
			key[col] = v
		}
		b, _ := json.Marshal(key)
		if _, e := tx.ExecContext(ctx, q, run, t.Name, string(b)); e != nil {
			return e
		}
	}
	return nil
}
func manifestRowsSQL(a adapters.Adapter) string {
	if a.Dialect() == "postgres" {
		return `INSERT INTO _seedflux_run_rows (run_id,table_name,key_json) VALUES ($1,$2,$3)`
	}
	return `INSERT INTO _seedflux_run_rows (run_id,table_name,key_json) VALUES (?,?,?)`
}
func manifestSQL(a adapters.Adapter) string {
	if a.Dialect() == "postgres" {
		return `INSERT INTO _seedflux_runs (run_id,created_at,schema_fingerprint,seed,counts) VALUES ($1,$2,$3,$4,$5)`
	}
	return `INSERT INTO _seedflux_runs (run_id,created_at,schema_fingerprint,seed,counts) VALUES (?,?,?,?,?)`
}
func Validate(ctx context.Context, a adapters.Adapter, s *schema.Schema) (map[string]int, error) {
	out := map[string]int{"foreign_keys": 0, "unique": 0, "checks": 0}
	if a.Dialect() == "sqlite" {
		r, e := a.DB().QueryContext(ctx, "PRAGMA foreign_key_check")
		if e != nil {
			return nil, e
		}
		defer r.Close()
		for r.Next() {
			out["foreign_keys"]++
		}
	}
	for _, t := range s.Tables {
		for _, u := range t.Uniques {
			if len(u) == 0 {
				continue
			}
			q := make([]string, len(u))
			for i, x := range u {
				q[i] = a.Quote(x)
			}
			var bad int
			e := a.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM (SELECT "+strings.Join(q, ",")+" FROM "+a.Quote(t.Name)+" GROUP BY "+strings.Join(q, ",")+" HAVING COUNT(*)>1)").Scan(&bad)
			if e != nil {
				return nil, e
			}
			out["unique"] += bad
		}
	}
	return out, nil
}
func Cleanup(ctx context.Context, a adapters.Adapter, s *schema.Schema, runID string) error {
	p, e := Plan(s)
	if e != nil {
		return e
	}
	tx, e := a.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var exists string
	if e = tx.QueryRowContext(ctx, placeholder(a, `SELECT run_id FROM _seedflux_runs WHERE run_id=?`, `SELECT run_id FROM _seedflux_runs WHERE run_id=$1`), runID).Scan(&exists); e != nil {
		return fmt.Errorf("run %s is not known: %w", runID, e)
	}
	for i := len(p.Order) - 1; i >= 0; i-- {
		t := s.Table(p.Order[i])
		if t == nil || len(t.PrimaryKey) == 0 {
			continue
		}
		rows, e := tx.QueryContext(ctx, placeholder(a, `SELECT key_json FROM _seedflux_run_rows WHERE run_id=? AND table_name=?`, `SELECT key_json FROM _seedflux_run_rows WHERE run_id=$1 AND table_name=$2`), runID, t.Name)
		if e != nil {
			return e
		}
		for rows.Next() {
			var raw string
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return e
			}
			var key map[string]any
			if e = json.Unmarshal([]byte(raw), &key); e != nil {
				rows.Close()
				return e
			}
			where := make([]string, 0, len(t.PrimaryKey))
			args := make([]any, 0, len(t.PrimaryKey))
			for j, col := range t.PrimaryKey {
				where = append(where, a.Quote(col)+"="+param(a, j+1))
				args = append(args, key[col])
			}
			if _, e = tx.ExecContext(ctx, "DELETE FROM "+a.Quote(t.Name)+" WHERE "+strings.Join(where, " AND "), args...); e != nil {
				rows.Close()
				return e
			}
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return e
		}
		rows.Close()
	}
	if _, e = tx.ExecContext(ctx, placeholder(a, `DELETE FROM _seedflux_run_rows WHERE run_id=?`, `DELETE FROM _seedflux_run_rows WHERE run_id=$1`), runID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, placeholder(a, `DELETE FROM _seedflux_runs WHERE run_id=?`, `DELETE FROM _seedflux_runs WHERE run_id=$1`), runID); e != nil {
		return e
	}
	return tx.Commit()
}
func param(a adapters.Adapter, n int) string {
	if a.Dialect() == "postgres" {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}
func placeholder(a adapters.Adapter, normal, pg string) string {
	if a.Dialect() == "postgres" {
		return pg
	}
	return normal
}
func ExportSQL(s *schema.Schema, c *config.Config) string {
	p, _ := Plan(s)
	var b strings.Builder
	for _, n := range p.Order {
		t := s.Table(n)
		if t == nil {
			continue
		}
		b.WriteString("-- " + n + "\n")
	}
	return b.String()
}
func SortedCounts(x map[string]int) []string {
	ks := make([]string, 0, len(x))
	for k := range x {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
