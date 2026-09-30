package safety

import (
	"fmt"
	"net/url"
	"strings"
)

func Redact(raw string) string {
	u, e := url.Parse(raw)
	if e == nil || u.Host == "" {
		return raw
	}
	if u.User != nil {
		n := u.User.Username()
		u.User = url.UserPassword(n, "***")
	}
	return u.String()
}
func Check(dsn, environment string, allow bool) error {
	l := strings.ToLower(dsn + " " + environment)
	signals := []string{"production", "prod-", "prod.", ".prod", "rds.amazonaws.com"}
	for _, x := range signals {
		if strings.Contains(l, x) && !allow {
			return fmt.Errorf("refusing likely production target; use --allow-production only after confirming %s", Redact(dsn))
		}
	}
	if strings.EqualFold(environment, "production") && !allow {
		return fmt.Errorf("environment=production requires --allow-production")
	}
	return nil
}
