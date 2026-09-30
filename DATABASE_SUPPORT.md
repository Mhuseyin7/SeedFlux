# Database support

| Database | Driver | Introspection |
| --- | --- | --- |
| SQLite | modernc.org/sqlite | tables, columns, PK/FK, unique indexes, generated columns, checks |
| PostgreSQL | pgx | tables, columns, PK/FK, unique constraints, defaults, generated columns |
| MySQL/MariaDB | go-sql-driver/mysql | tables, columns, PK/FK, unique constraints, auto increment |

Adapters are real driver-backed implementations, not mocks. Run database
integration suites with Docker/CI before declaring support for a server version.
