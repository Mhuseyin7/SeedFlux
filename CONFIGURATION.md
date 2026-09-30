# Configuration

```yaml
version: 1
environment: development
seed: 42
database:
  dialect: sqlite
  url: ./development.db
tables:
  users:
    count: 100
    columns:
      email: { generator: email }
      country_code:
        weighted: { TR: 0.4, DE: 0.2, GB: 0.2, US: 0.2 }
  orders:
    count:
      per_parent: { table: users, min: 0, max: 8 }
```

The parser rejects unsupported versions, invalid null probabilities, negative
counts and unsafe expressions. The safe expression vocabulary is reserved for
`copy`, `concat`, `multiply`, `date_after`, and `sum_children`; arbitrary code
evaluation is never permitted.
