package writer

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/seedflux/seedflux/internal/adapters"
	"github.com/seedflux/seedflux/internal/schema"
	"strings"
)

type Row map[string]any

func Insert(ctx context.Context, tx *sql.Tx, a adapters.Adapter, t *schema.Table, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	const batch = 500
	for start := 0; start < len(rows); start += batch {
		end := start + batch
		if end > len(rows) {
			end = len(rows)
		}
		if e := insertBatch(ctx, tx, a, t, rows[start:end]); e != nil {
			return e
		}
	}
	return nil
}
func insertBatch(ctx context.Context, tx *sql.Tx, a adapters.Adapter, t *schema.Table, rows []Row) error {
	var cols []string
	for _, c := range t.Columns {
		if c.Generated || c.Identity {
			continue
		}
		for _, r := range rows {
			if _, ok := r[c.Name]; ok {
				cols = append(cols, c.Name)
				break
			}
		}
	}
	if len(cols) == 0 {
		return nil
	}
	var groups []string
	var args []any
	for _, r := range rows {
		p := make([]string, len(cols))
		for i, c := range cols {
			p[i] = "?"
			args = append(args, r[c])
		}
		groups = append(groups, "("+strings.Join(p, ",")+")")
	}
	if a.Dialect() == "postgres" {
		n := 0
		for i := range groups {
			for range cols {
				n++
				needle := "?"
				pos := strings.Index(groups[i], needle)
				groups[i] = groups[i][:pos] + fmt.Sprintf("$%d", n) + groups[i][pos+1:]
			}
		}
	}
	qcols := make([]string, len(cols))
	for i, c := range cols {
		qcols[i] = a.Quote(c)
	}
	_, e := tx.ExecContext(ctx, "INSERT INTO "+a.Quote(t.Name)+" ("+strings.Join(qcols, ",")+") VALUES "+strings.Join(groups, ","), args...)
	return e
}
