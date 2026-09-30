# Architecture

`internal/schema` is the normalized, database-neutral model. `internal/adapters`
contains the only dialect-specific introspection and SQL quoting code.
`internal/graph` produces parent-before-child plans; it identifies cyclic
components explicitly. `internal/generator` derives independent deterministic
streams from `(seed, table, column, row)`. `internal/writer` inserts bounded
batches. `internal/engine` coordinates manifests, planning, generation and
validation. `internal/safety` owns target redaction and production safeguards.

The CLI is deliberately thin and does not embed schema rules.
