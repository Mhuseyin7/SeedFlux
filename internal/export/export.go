// Package export renders deterministic generated rows without connecting writes.
package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/seedflux/seedflux/internal/adapters"
	"github.com/seedflux/seedflux/internal/writer"
)

func SQL(w io.Writer, a adapters.Adapter, table string, rows []writer.Row) error {
	for _, row := range rows {
		cols := make([]string, 0, len(row))
		for col := range row {
			cols = append(cols, col)
		}
		sortStrings(cols)
		values := make([]string, len(cols))
		for i, col := range cols {
			values[i] = literal(row[col])
		}
		quoted := make([]string, len(cols))
		for i, col := range cols {
			quoted[i] = a.Quote(col)
		}
		if _, err := fmt.Fprintf(w, "INSERT INTO %s (%s) VALUES (%s);\n", a.Quote(table), strings.Join(quoted, ", "), strings.Join(values, ", ")); err != nil {
			return err
		}
	}
	return nil
}

func CSV(w io.Writer, table string, rows []writer.Row) error {
	if len(rows) == 0 {
		return nil
	}
	cw := csv.NewWriter(w)
	defer cw.Flush()
	cols := make([]string, 0, len(rows[0]))
	for col := range rows[0] {
		cols = append(cols, col)
	}
	sortStrings(cols)
	if err := cw.Write(cols); err != nil {
		return err
	}
	for _, row := range rows {
		record := make([]string, len(cols))
		for i, col := range cols {
			record[i] = fmt.Sprint(row[col])
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	return cw.Error()
}

func JSONL(w io.Writer, table string, rows []writer.Row) error {
	for _, row := range rows {
		payload := map[string]any{"table": table, "row": row}
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintln(w, string(b)); err != nil {
			return err
		}
	}
	return nil
}

func literal(v any) string {
	if v == nil {
		return "NULL"
	}
	switch x := v.(type) {
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case int, int64, float64:
		return fmt.Sprint(x)
	case []byte:
		return "X'" + fmt.Sprintf("%x", x) + "'"
	default:
		return "'" + strings.ReplaceAll(fmt.Sprint(v), "'", "''") + "'"
	}
}
func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
