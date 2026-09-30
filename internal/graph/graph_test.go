package graph

import (
	"github.com/seedflux/seedflux/internal/schema"
	"testing"
)

func TestBuildOrdersParentsFirst(t *testing.T) {
	s := &schema.Schema{Tables: []*schema.Table{{Name: "parents"}, {Name: "children", ForeignKeys: []*schema.ForeignKey{{RefTable: "parents"}}}}}
	p, e := Build(s)
	if e != nil || len(p.Order) != 2 || p.Order[0] != "parents" {
		t.Fatalf("unexpected plan %#v, %v", p, e)
	}
}
