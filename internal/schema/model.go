package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Schema is a database-neutral, normalized description of a database schema.
type Schema struct {
	Dialect string   `json:"dialect"`
	Tables  []*Table `json:"tables"`
}
type Table struct {
	Schema      string        `json:"schema,omitempty"`
	Name        string        `json:"name"`
	Columns     []*Column     `json:"columns"`
	PrimaryKey  []string      `json:"primary_key,omitempty"`
	Uniques     [][]string    `json:"uniques,omitempty"`
	ForeignKeys []*ForeignKey `json:"foreign_keys,omitempty"`
	Checks      []string      `json:"checks,omitempty"`
}
type Column struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Nullable  bool     `json:"nullable"`
	Default   string   `json:"default,omitempty"`
	Generated bool     `json:"generated,omitempty"`
	Identity  bool     `json:"identity,omitempty"`
	Enum      []string `json:"enum,omitempty"`
}
type ForeignKey struct {
	Name       string   `json:"name,omitempty"`
	Columns    []string `json:"columns"`
	RefTable   string   `json:"ref_table"`
	RefColumns []string `json:"ref_columns"`
	Deferrable bool     `json:"deferrable,omitempty"`
}

func (s *Schema) Table(name string) *Table {
	for _, t := range s.Tables {
		if t.Name == name {
			return t
		}
	}
	return nil
}
func (t *Table) Column(name string) *Column {
	for _, c := range t.Columns {
		if c.Name == name {
			return c
		}
	}
	return nil
}
func (s *Schema) Fingerprint() string {
	var p []string
	for _, t := range s.Tables {
		p = append(p, t.Name)
		for _, c := range t.Columns {
			p = append(p, c.Name+":"+strings.ToLower(c.Type))
		}
		for _, f := range t.ForeignKeys {
			p = append(p, strings.Join(f.Columns, ",")+">"+f.RefTable+":"+strings.Join(f.RefColumns, ","))
		}
	}
	sort.Strings(p)
	h := sha256.Sum256([]byte(strings.Join(p, "|")))
	return hex.EncodeToString(h[:])
}
func (s *Schema) FilterMigrations(include bool) {
	if include {
		return
	}
	out := s.Tables[:0]
	for _, t := range s.Tables {
		n := strings.ToLower(t.Name)
		if n == "alembic_version" || n == "django_migrations" || n == "knex_migrations" || n == "knex_migrations_lock" || n == "sequelizemeta" || n == "flyway_schema_history" || n == "__efmigrationshistory" || strings.Contains(n, "prisma_migrations") {
			continue
		}
		out = append(out, t)
	}
	s.Tables = out
}
