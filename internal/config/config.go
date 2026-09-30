package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

type Config struct {
	Version     int                       `yaml:"version"`
	Database    Database                  `yaml:"database"`
	Seed        int64                     `yaml:"seed"`
	Environment string                    `yaml:"environment"`
	Tables      map[string]Table          `yaml:"tables"`
	Profiles    map[string]map[string]int `yaml:"profiles"`
}
type Database struct {
	Dialect string `yaml:"dialect"`
	URL     string `yaml:"url"`
}
type Table struct {
	Count   Count             `yaml:"count"`
	Columns map[string]Column `yaml:"columns"`
}
type Count struct {
	Value     int
	PerParent *PerParent
}
type PerParent struct {
	Table string `yaml:"table"`
	Min   int    `yaml:"min"`
	Max   int    `yaml:"max"`
}
type Column struct {
	Generator       string             `yaml:"generator"`
	NullProbability float64            `yaml:"null_probability"`
	Weighted        map[string]float64 `yaml:"weighted"`
	After           string             `yaml:"after"`
	Expression      string             `yaml:"expression"`
}

func (c *Count) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		var i int
		if e := n.Decode(&i); e != nil {
			return e
		}
		c.Value = i
		return nil
	}
	var x struct {
		PerParent *PerParent `yaml:"per_parent"`
	}
	if e := n.Decode(&x); e != nil {
		return e
	}
	c.PerParent = x.PerParent
	return nil
}
func Load(path string) (*Config, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var c Config
	if e = yaml.Unmarshal(b, &c); e != nil {
		return nil, e
	}
	if c.Version != 1 {
		return nil, fmt.Errorf("unsupported config version %d", c.Version)
	}
	for t, v := range c.Tables {
		if v.Count.Value < 0 {
			return nil, fmt.Errorf("tables.%s.count must be non-negative", t)
		}
		if v.Count.PerParent != nil && (v.Count.PerParent.Table == "" || v.Count.PerParent.Min < 0 || v.Count.PerParent.Max < v.Count.PerParent.Min) {
			return nil, fmt.Errorf("invalid per_parent count for %s", t)
		}
		for n, col := range v.Columns {
			if col.NullProbability < 0 || col.NullProbability > 1 {
				return nil, fmt.Errorf("tables.%s.columns.%s null_probability must be 0..1", t, n)
			}
			if col.Expression != "" && !validExpression(col.Expression) {
				return nil, fmt.Errorf("unsupported safe expression %q", col.Expression)
			}
		}
	}
	return &c, nil
}
func validExpression(s string) bool {
	for _, p := range []string{"copy(", "concat(", "multiply(", "date_after(", "sum_children("} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
func Default() *Config {
	return &Config{Version: 1, Seed: 42, Environment: "development", Tables: map[string]Table{}}
}

// ApplyProfile overlays table cardinalities without mutating generator rules.
func (c *Config) ApplyProfile(name string) error {
	if name == "" {
		return nil
	}
	p, ok := c.Profiles[name]
	if !ok {
		return fmt.Errorf("profile %q is not defined", name)
	}
	for table, count := range p {
		if count < 0 {
			return fmt.Errorf("profile %q has negative count for %s", name, table)
		}
		t := c.Tables[table]
		t.Count = Count{Value: count}
		c.Tables[table] = t
	}
	return nil
}
