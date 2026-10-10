package safety

import (
	"strings"
	"testing"
)

func TestRedactCredentials(t *testing.T) {
	for _, raw := range []string{
		"postgres://alice:secret@example.test/app",
		"user=alice password=secret host=db.example.test",
		"mysql://alice:secret@db.example.test/app?token=abc",
	} {
		got := Redact(raw)
		if got == raw || strings.Contains(got, "secret") || strings.Contains(got, "abc") {
			t.Fatalf("credentials were not fully redacted: %q", got)
		}
	}
}
