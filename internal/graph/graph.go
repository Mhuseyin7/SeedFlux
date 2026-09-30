package graph

import (
	"fmt"
	"github.com/seedflux/seedflux/internal/schema"
	"sort"
)

type Plan struct {
	Order  []string
	Cyclic [][]string
}

func Build(s *schema.Schema) (Plan, error) {
	deps := map[string]map[string]bool{}
	rev := map[string][]string{}
	for _, t := range s.Tables {
		deps[t.Name] = map[string]bool{}
		for _, f := range t.ForeignKeys {
			if f.RefTable != t.Name {
				deps[t.Name][f.RefTable] = true
				rev[f.RefTable] = append(rev[f.RefTable], t.Name)
			}
		}
	}
	var ready []string
	for n, d := range deps {
		if len(d) == 0 {
			ready = append(ready, n)
		}
	}
	sort.Strings(ready)
	out := Plan{}
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		out.Order = append(out.Order, n)
		for _, child := range rev[n] {
			delete(deps[child], n)
			if len(deps[child]) == 0 {
				ready = append(ready, child)
				sort.Strings(ready)
			}
		}
	}
	if len(out.Order) == len(s.Tables) {
		return out, nil
	}
	var cyc []string
	for n, d := range deps {
		if len(d) > 0 {
			cyc = append(cyc, n)
		}
	}
	sort.Strings(cyc)
	out.Cyclic = append(out.Cyclic, cyc)
	out.Order = append(out.Order, cyc...)
	return out, nil
}
func (p Plan) String() string {
	if len(p.Order) == 0 {
		return "No seedable tables."
	}
	o := ""
	for i, n := range p.Order {
		o += fmt.Sprintf("%d. %s\n", i+1, n)
	}
	for _, c := range p.Cyclic {
		o += fmt.Sprintf("cyclic pass: %v\n", c)
	}
	return o
}
