package generator

import (
	"github.com/seedflux/seedflux/internal/config"
	"github.com/seedflux/seedflux/internal/schema"
	"testing"
)

func TestDeterministicEmail(t *testing.T) {
	c := &schema.Column{Name: "email", Type: "text"}
	tb := &schema.Table{Name: "users"}
	a, _ := New(42).Value(tb, c, 4, config.Column{})
	b, _ := New(42).Value(tb, c, 4, config.Column{})
	if a != b {
		t.Fatal("expected deterministic value")
	}
}
