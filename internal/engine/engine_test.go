package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/seedflux/seedflux/internal/adapters"
	"github.com/seedflux/seedflux/internal/config"
)

func TestSQLiteFixtureGenerateValidateAndCleanup(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fixture.db")
	a, err := adapters.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB().Close()
	b, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "sqlite", "ecommerce.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(b), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err = a.DB().ExecContext(ctx, statement); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	s, err := a.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Tables["users"] = config.Table{Count: config.Count{Value: 4}}
	c.Tables["products"] = config.Table{Count: config.Count{Value: 3}}
	c.Tables["categories"] = config.Table{Count: config.Count{Value: 2}}
	c.Tables["addresses"] = config.Table{Count: config.Count{Value: 4}}
	c.Tables["orders"] = config.Table{Count: config.Count{Value: 5}}
	c.Tables["order_items"] = config.Table{Count: config.Count{Value: 6}}
	c.Tables["payments"] = config.Table{Count: config.Count{Value: 5}}
	r, err := Generate(ctx, a, s, c, false)
	if err != nil {
		t.Fatal(err)
	}
	v, err := Validate(ctx, a, s)
	if err != nil {
		t.Fatal(err)
	}
	if v["foreign_keys"] != 0 || v["unique"] != 0 {
		t.Fatalf("invalid output: %#v", v)
	}
	if err := Cleanup(ctx, a, s, r.RunID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := a.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil || count != 0 {
		t.Fatalf("cleanup count=%d err=%v", count, err)
	}
}

func TestSQLiteFixtureConfiguration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fixture.db")
	a, err := adapters.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.DB().Close()
	b, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "sqlite", "ecommerce.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(b), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err = a.DB().ExecContext(ctx, statement); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	c, err := config.Load(filepath.Join("..", "..", "fixtures", "seedflux.yml"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := a.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Generate(ctx, a, s, c, false); err != nil {
		t.Fatal(err)
	}
}
