package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/seedflux/seedflux/internal/schema"
	_ "modernc.org/sqlite"
)

// Adapter isolates dialect-specific metadata and SQL behavior.
type Adapter interface {
	Dialect() string
	DB() *sql.DB
	Inspect(context.Context) (*schema.Schema, error)
	Quote(string) string
	Begin(context.Context) (*sql.Tx, error)
}

func Open(dsn, dialect string) (Adapter, error) {
	d := strings.ToLower(dialect)
	if d == "" {
		switch {
		case strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://"):
			d = "postgres"
		case strings.HasPrefix(dsn, "mysql://"):
			d = "mysql"
		default:
			d = "sqlite"
		}
	}
	switch d {
	case "sqlite", "sqlite3":
		return OpenSQLite(dsn)
	case "postgres", "postgresql":
		return OpenPostgres(dsn)
	case "mysql", "mariadb":
		return OpenMySQL(dsn)
	default:
		return nil, fmt.Errorf("unsupported dialect %q", dialect)
	}
}
