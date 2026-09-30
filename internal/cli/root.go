package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/seedflux/seedflux/internal/adapters"
	"github.com/seedflux/seedflux/internal/config"
	"github.com/seedflux/seedflux/internal/engine"
	dataexport "github.com/seedflux/seedflux/internal/export"
	"github.com/seedflux/seedflux/internal/safety"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var version = "dev"

func Execute() {
	root := &cobra.Command{Use: "seedflux", Short: "Schema-aware synthetic data generation.", Version: version}
	root.AddCommand(initCmd(), inspectCmd(), planCmd(), generateCmd(), validateCmd(), runsCmd(), cleanupCmd(), doctorCmd(), exportCmd())
	if e := root.Execute(); e != nil {
		os.Exit(1)
	}
}
func common(c *cobra.Command) {
	c.Flags().String("url", "", "database DSN")
	c.Flags().String("dialect", "", "postgres, mysql, or sqlite")
	c.Flags().String("config", "seedflux.yml", "configuration file")
}
func open(c *cobra.Command) (adapters.Adapter, *config.Config, error) {
	url, _ := c.Flags().GetString("url")
	dialect, _ := c.Flags().GetString("dialect")
	path, _ := c.Flags().GetString("config")
	cfg := config.Default()
	if _, e := os.Stat(path); e == nil {
		x, e := config.Load(path)
		if e != nil {
			return nil, nil, e
		}
		cfg = x
	}
	if url == "" {
		url = cfg.Database.URL
	}
	if dialect == "" {
		dialect = cfg.Database.Dialect
	}
	if url == "" {
		return nil, nil, fmt.Errorf("--url or database.url is required")
	}
	a, e := adapters.Open(url, dialect)
	return a, cfg, e
}
func initCmd() *cobra.Command {
	c := &cobra.Command{Use: "init", Short: "write a starter seedflux.yml", RunE: func(c *cobra.Command, _ []string) error {
		p, _ := c.Flags().GetString("config")
		if _, e := os.Stat(p); e == nil {
			return fmt.Errorf("%s already exists", p)
		}
		return os.WriteFile(p, []byte("version: 1\nenvironment: development\nseed: 42\ndatabase:\n  dialect: sqlite\n  url: ./development.db\ntables: {}\n"), 0644)
	}}
	c.Flags().String("config", "seedflux.yml", "configuration file")
	return c
}
func inspectCmd() *cobra.Command {
	c := &cobra.Command{Use: "inspect", Short: "inspect database schema", RunE: func(c *cobra.Command, _ []string) error {
		a, _, e := open(c)
		if e != nil {
			return e
		}
		defer a.DB().Close()
		s, e := a.Inspect(context.Background())
		if e != nil {
			return e
		}
		s.FilterMigrations(false)
		j, _ := c.Flags().GetBool("json")
		if j {
			b, _ := json.MarshalIndent(s, "", "  ")
			fmt.Println(string(b))
			return nil
		}
		for _, t := range s.Tables {
			fmt.Println("\n" + t.Name)
			for _, x := range t.Columns {
				extra := ""
				if x.Nullable {
					extra = " nullable"
				}
				fmt.Printf("├── %s %s%s\n", x.Name, x.Type, extra)
			}
			for _, f := range t.ForeignKeys {
				fmt.Printf("├── %s → %s.%s\n", strings.Join(f.Columns, ","), f.RefTable, strings.Join(f.RefColumns, ","))
			}
		}
		return nil
	}}
	common(c)
	c.Flags().Bool("json", false, "emit JSON")
	return c
}
func planCmd() *cobra.Command {
	c := &cobra.Command{Use: "plan", Short: "show dependency-ordered generation plan", RunE: func(c *cobra.Command, _ []string) error {
		a, _, e := open(c)
		if e != nil {
			return e
		}
		defer a.DB().Close()
		s, e := a.Inspect(context.Background())
		if e != nil {
			return e
		}
		s.FilterMigrations(false)
		p, e := engine.Plan(s)
		if e == nil {
			fmt.Print(p.String())
		}
		return e
	}}
	common(c)
	return c
}
func generateCmd() *cobra.Command {
	c := &cobra.Command{Use: "generate", Short: "generate relational synthetic data", RunE: func(c *cobra.Command, _ []string) error {
		a, cfg, e := open(c)
		if e != nil {
			return e
		}
		defer a.DB().Close()
		seed, _ := c.Flags().GetInt64("seed")
		if c.Flags().Changed("seed") {
			cfg.Seed = seed
		}
		profile, _ := c.Flags().GetString("profile")
		if e = cfg.ApplyProfile(profile); e != nil {
			return e
		}
		for _, spec := range mustStrings(c.Flags().GetStringSlice("table")) {
			parts := strings.SplitN(spec, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("--table must be table=count")
			}
			var n int
			if _, err := fmt.Sscanf(parts[1], "%d", &n); err != nil || n < 0 {
				return fmt.Errorf("invalid count %q", parts[1])
			}
			t := cfg.Tables[parts[0]]
			t.Count = config.Count{Value: n}
			cfg.Tables[parts[0]] = t
		}
		for flag, table := range map[string]string{"users": "users", "products": "products", "orders": "orders", "order-items": "order_items", "payments": "payments", "addresses": "addresses", "categories": "categories"} {
			if c.Flags().Changed(flag) {
				n, _ := c.Flags().GetInt(flag)
				t := cfg.Tables[table]
				t.Count = config.Count{Value: n}
				cfg.Tables[table] = t
			}
		}
		allow, _ := c.Flags().GetBool("allow-production")
		url, _ := c.Flags().GetString("url")
		if url == "" {
			url = cfg.Database.URL
		}
		if e = safety.Check(url, cfg.Environment, allow); e != nil {
			return e
		}
		dry, _ := c.Flags().GetBool("dry-run")
		s, e := a.Inspect(context.Background())
		if e != nil {
			return e
		}
		s.FilterMigrations(false)
		r, e := engine.Generate(context.Background(), a, s, cfg, dry)
		if e != nil {
			return e
		}
		if dry {
			fmt.Println("Dry run; no writes.")
			fmt.Print(r.Plan.String())
		}
		for _, k := range engine.SortedCounts(r.Counts) {
			fmt.Printf("%d %s\n", r.Counts[k], k)
		}
		if !dry {
			v, e := engine.Validate(context.Background(), a, s)
			if e != nil {
				return e
			}
			fmt.Printf("Run: %s\nForeign-key violations: %d\nUnique violations: %d\nCheck violations: %d\n", r.RunID, v["foreign_keys"], v["unique"], v["checks"])
		}
		return nil
	}}
	common(c)
	c.Flags().Int64("seed", 42, "deterministic seed")
	c.Flags().Bool("dry-run", false, "plan without writes")
	c.Flags().Bool("allow-production", false, "explicitly allow a production-like target")
	c.Flags().StringSlice("table", nil, "table=count override; repeatable")
	c.Flags().String("profile", "", "named configuration profile")
	for _, flag := range []string{"users", "products", "orders", "order-items", "payments", "addresses", "categories"} {
		c.Flags().Int(flag, 0, "override generated row count")
	}
	return c
}
func validateCmd() *cobra.Command {
	c := &cobra.Command{Use: "validate", Short: "validate foreign keys and unique constraints", RunE: func(c *cobra.Command, _ []string) error {
		a, _, e := open(c)
		if e != nil {
			return e
		}
		defer a.DB().Close()
		s, e := a.Inspect(context.Background())
		if e != nil {
			return e
		}
		s.FilterMigrations(false)
		v, e := engine.Validate(context.Background(), a, s)
		if e != nil {
			return e
		}
		fmt.Printf("Foreign-key violations: %d\nUnique violations: %d\nCheck violations: %d\n", v["foreign_keys"], v["unique"], v["checks"])
		if v["foreign_keys"]+v["unique"]+v["checks"] > 0 {
			return fmt.Errorf("validation failed")
		}
		return nil
	}}
	common(c)
	return c
}
func runsCmd() *cobra.Command {
	c := &cobra.Command{Use: "runs", Short: "list local SeedFlux run manifests", RunE: func(c *cobra.Command, _ []string) error {
		a, _, e := open(c)
		if e != nil {
			return e
		}
		defer a.DB().Close()
		r, e := a.DB().QueryContext(context.Background(), "SELECT run_id,created_at,seed FROM _seedflux_runs ORDER BY created_at DESC")
		if e != nil {
			return fmt.Errorf("no run manifests: %w", e)
		}
		defer r.Close()
		for r.Next() {
			var id, at string
			var seed int64
			r.Scan(&id, &at, &seed)
			fmt.Printf("%s  %s  seed=%d\n", id, at, seed)
		}
		return r.Err()
	}}
	common(c)
	return c
}
func cleanupCmd() *cobra.Command {
	c := &cobra.Command{Use: "cleanup <run-id>", Args: cobra.ExactArgs(1), Short: "safely remove rows captured for a run", RunE: func(c *cobra.Command, args []string) error {
		a, _, e := open(c)
		if e != nil {
			return e
		}
		defer a.DB().Close()
		s, e := a.Inspect(context.Background())
		if e != nil {
			return e
		}
		return engine.Cleanup(context.Background(), a, s, args[0])
	}}
	common(c)
	return c
}
func doctorCmd() *cobra.Command {
	c := &cobra.Command{Use: "doctor", Short: "verify connectivity and safety posture", RunE: func(c *cobra.Command, _ []string) error {
		a, cfg, e := open(c)
		if e != nil {
			return e
		}
		defer a.DB().Close()
		if e = a.DB().PingContext(context.Background()); e != nil {
			return e
		}
		url, _ := c.Flags().GetString("url")
		if url == "" {
			url = cfg.Database.URL
		}
		if e = safety.Check(url, cfg.Environment, false); e != nil {
			fmt.Println("Safety:", e)
		} else {
			fmt.Println("Safety: development-safe")
		}
		fmt.Printf("Connected: %s (%s)\n", safety.Redact(url), a.Dialect())
		return nil
	}}
	common(c)
	return c
}
func exportCmd() *cobra.Command {
	c := &cobra.Command{Use: "export", Short: "export FK-consistent synthetic data without inserting", RunE: func(c *cobra.Command, _ []string) error {
		format, _ := c.Flags().GetString("format")
		if format != "sql" && format != "csv" && format != "jsonl" {
			return fmt.Errorf("format must be sql, csv, or jsonl")
		}
		a, cfg, e := open(c)
		if e != nil {
			return e
		}
		defer a.DB().Close()
		profile, _ := c.Flags().GetString("profile")
		if e = cfg.ApplyProfile(profile); e != nil {
			return e
		}
		s, e := a.Inspect(context.Background())
		if e != nil {
			return e
		}
		poolsPlan, rows, _, e := engine.Rows(s, cfg)
		if e != nil {
			return e
		}
		for _, name := range poolsPlan.Order {
			switch format {
			case "sql":
				e = dataexport.SQL(os.Stdout, a, name, rows[name])
			case "csv":
				e = dataexport.CSV(os.Stdout, name, rows[name])
			case "jsonl":
				e = dataexport.JSONL(os.Stdout, name, rows[name])
			}
			if e != nil {
				return e
			}
		}
		return nil
	}}
	common(c)
	c.Flags().String("format", "sql", "sql, csv, or jsonl")
	c.Flags().String("profile", "", "named configuration profile")
	return c
}
func mustStrings(values []string, err error) []string {
	if err != nil {
		return nil
	}
	return values
}
