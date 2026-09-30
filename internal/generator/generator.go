package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/seedflux/seedflux/internal/config"
	"github.com/seedflux/seedflux/internal/schema"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
)

type ValueSource struct {
	Seed uint64
	At   time.Time
}

func New(seed int64) ValueSource {
	return ValueSource{Seed: uint64(seed), At: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
}
func (v ValueSource) rng(table, col string, row int) *rand.Rand {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s:%d", v.Seed, table, col, row)))
	return rand.New(rand.NewPCG(uint64From(h[:8]), uint64From(h[8:16])))
}
func uint64From(b []byte) uint64 {
	var n uint64
	for _, x := range b {
		n = n<<8 | uint64(x)
	}
	return n
}
func (v ValueSource) Value(t *schema.Table, c *schema.Column, row int, over config.Column) (any, error) {
	r := v.rng(t.Name, c.Name, row)
	if c.Nullable && over.NullProbability > 0 && r.Float64() < over.NullProbability {
		return nil, nil
	}
	g := over.Generator
	if g == "" {
		g = infer(c)
	}
	if len(c.Enum) > 0 {
		g = "enum"
	}
	switch g {
	case "uuid":
		h := sha256.Sum256([]byte(fmt.Sprintf("%d/%s/%s/%d", v.Seed, t.Name, c.Name, row)))
		x := hex.EncodeToString(h[:])
		return x[:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:32], nil
	case "email":
		return fmt.Sprintf("user%06d_%s@example.test", row, t.Name), nil
	case "username":
		return fmt.Sprintf("%s_%06d", t.Name, row), nil
	case "first_name":
		return []string{"Ada", "Deniz", "Mina", "Arda", "Lina"}[row%5], nil
	case "last_name":
		return []string{"Yılmaz", "Kaya", "Smith", "Chen", "Ivanov"}[row%5], nil
	case "phone":
		return fmt.Sprintf("+1555%07d", row%10000000), nil
	case "url":
		return fmt.Sprintf("https://example.test/%s/%d", t.Name, row), nil
	case "slug":
		return fmt.Sprintf("%s-%d", strings.ReplaceAll(t.Name, "_", "-"), row), nil
	case "country", "country_code":
		return weighted(over, []string{"TR", "DE", "GB", "US"}, r), nil
	case "currency":
		return "USD", nil
	case "boolean":
		return r.IntN(2) == 0, nil
	case "date":
		return v.At.AddDate(0, 0, row%365).Format("2006-01-02"), nil
	case "datetime":
		return v.At.Add(time.Duration(row%31536000) * time.Second).Format(time.RFC3339), nil
	case "enum":
		return c.Enum[row%len(c.Enum)], nil
	case "json":
		return fmt.Sprintf(`{"synthetic":true,"row":%d}`, row), nil
	case "bytes":
		return []byte(fmt.Sprintf("seedflux-%d", row)), nil
	case "ip":
		return fmt.Sprintf("198.51.100.%d", 1+row%254), nil
	case "decimal":
		return fmt.Sprintf("%.2f", 0.01+r.Float64()*999.99), nil
	case "integer":
		return int64(1 + row), nil
	case "string":
		return fmt.Sprintf("%s %d", strings.ReplaceAll(c.Name, "_", " "), row), nil
	default:
		return nil, fmt.Errorf("unknown generator %q for %s.%s", g, t.Name, c.Name)
	}
}
func weighted(c config.Column, def []string, r *rand.Rand) string {
	if len(c.Weighted) == 0 {
		return def[r.IntN(len(def))]
	}
	x := r.Float64()
	var total, sum float64
	for _, v := range c.Weighted {
		total += v
	}
	for k, v := range c.Weighted {
		sum += v / total
		if x <= sum {
			return k
		}
	}
	for k := range c.Weighted {
		return k
	}
	return ""
}
func infer(c *schema.Column) string {
	n := strings.ToLower(c.Name)
	typ := strings.ToLower(c.Type)
	switch {
	case strings.Contains(n, "email"):
		return "email"
	case n == "id" || strings.HasSuffix(n, "_id") && strings.Contains(typ, "uuid"):
		return "uuid"
	case strings.Contains(n, "first_name"):
		return "first_name"
	case strings.Contains(n, "last_name"):
		return "last_name"
	case strings.Contains(n, "phone"):
		return "phone"
	case strings.Contains(n, "url") || strings.Contains(n, "website"):
		return "url"
	case strings.Contains(n, "slug"):
		return "slug"
	case strings.Contains(n, "country"):
		return "country"
	case strings.Contains(n, "currency"):
		return "currency"
	case strings.Contains(n, "price") || strings.Contains(n, "amount") || strings.Contains(n, "decimal") || strings.Contains(typ, "numeric") || strings.Contains(typ, "decimal") || strings.Contains(typ, "real") || strings.Contains(typ, "double"):
		return "decimal"
	case strings.Contains(n, "created_at") || strings.Contains(n, "updated_at") || strings.Contains(typ, "timestamp") || strings.Contains(typ, "datetime"):
		return "datetime"
	case strings.Contains(typ, "bool"):
		return "boolean"
	case strings.Contains(typ, "int"):
		return "integer"
	case strings.Contains(typ, "json"):
		return "json"
	default:
		return "string"
	}
}
func ParseCount(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case string:
		i, _ := strconv.Atoi(x)
		return i
	}
	return 0
}
