# SeedFlux

> **Schema-aware synthetic data generation.**
>
> Relational database'ler için deterministic, realistic ve constraint-aware synthetic data generation platformu.

[![CI](https://github.com/Mhuseyin7/SeedFlux/actions/workflows/ci.yml/badge.svg)](https://github.com/Mhuseyin7/SeedFlux/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8.svg)](https://go.dev/)

SeedFlux açık kaynak kodlu bir software projesidir ve **[muhammedkoca.com.tr](https://muhammedkoca.com.tr)** tarafından geliştirilmiştir.

## Nedir?

SeedFlux, development ve test database'lerine bağlanır; gerçek schema'yı inspect eder, table dependency'lerini analiz eder ve foreign key, unique constraint, nullable alanlar, generated column'lar ile yaygın check constraint'lere uyumlu synthetic data üretir.

Klasik bir Faker wrapper değildir. Amaç sadece random data yazmak değil; relational integrity korunmuş, tekrar üretilebilir ve test senaryolarına uygun bir dataset oluşturmaktır.

```text
users ──┐
        ├── orders ── order_items
products ┘       └── payments
```

## Özellikler

- PostgreSQL, MySQL/MariaDB ve SQLite için driver-backed adapter architecture
- Database schema inspection: tables, columns, primary key, foreign key, unique index, defaults, generated columns ve SQLite check constraints
- Dependency graph ve topological generation plan
- Deterministic random generation (`--seed 42`)
- Semantic generator detection: `email`, `name`, `phone`, `url`, `slug`, `country`, `currency`, `price`, `created_at` ve daha fazlası
- UUID, integer, decimal, boolean, string, JSON, bytes, IP, enum, date/datetime generator'ları
- Single ve composite unique constraint desteği
- Parent-child cardinality ve named profile desteği
- `--dry-run` ile write olmadan plan/row count inceleme
- SQL, CSV ve JSONL export
- Foreign key ve unique validation
- Run manifest ve captured primary key'ler üzerinden safe cleanup
- Production-like connection target'ları varsayılan olarak reddeden safety guard
- Connection string redaction; password log'a yazılmaz

## Hızlı Başlangıç

### 1. Binary build edin

```bash
git clone https://github.com/Mhuseyin7/SeedFlux.git
cd SeedFlux
go build -o seedflux ./cmd/seedflux
```

Windows için:

```powershell
go build -o seedflux.exe .\cmd\seedflux
```

### 2. Configuration oluşturun

```bash
./seedflux init
```

`seedflux.yml` örneği:

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
      email:
        generator: email
      country_code:
        weighted:
          TR: 0.40
          DE: 0.20
          GB: 0.20
          US: 0.20

  orders:
    count:
      per_parent:
        table: users
        min: 1
        max: 4

profiles:
  demo:
    users: 50
  load-test:
    users: 100000
```

### 3. Schema'yı inceleyin

```bash
./seedflux inspect --url ./development.db --dialect sqlite
./seedflux inspect --url ./development.db --dialect sqlite --json
```

### 4. Generation plan'i görün

```bash
./seedflux plan --url ./development.db --dialect sqlite
```

### 5. Data üretin ve validate edin

```bash
./seedflux generate --url ./development.db --dialect sqlite --seed 42
./seedflux validate --url ./development.db --dialect sqlite
```

Hızlı table override örneği:

```bash
./seedflux generate --url ./development.db --dialect sqlite \
  --users 1000 --products 250 --orders 5000 --seed 42
```

Windows PowerShell örneği:

```powershell
.\seedflux.exe generate --url .\development.db --dialect sqlite --users 1000 --seed 42
```

## CLI Komutları

| Command | Açıklama |
| --- | --- |
| `seedflux init` | Starter `seedflux.yml` oluşturur. |
| `seedflux inspect` | Live schema'yı inspect eder. |
| `seedflux plan` | Parent-first generation plan gösterir. |
| `seedflux generate` | Synthetic relational dataset üretir. |
| `seedflux export` | Database'e insert etmeden SQL, CSV veya JSONL export üretir. |
| `seedflux validate` | FK ve unique constraint kontrollerini çalıştırır. |
| `seedflux runs` | Local generation manifest'lerini listeler. |
| `seedflux cleanup <run-id>` | Sadece bilinen run'a ait captured key'leri güvenle siler. |
| `seedflux doctor` | Connection ve safety posture kontrolü yapar. |

### Dry run

```bash
./seedflux generate --url ./development.db --dialect sqlite --dry-run
```

Bu modda database'e write yapılmaz; generation order ve estimated row count gösterilir.

### Export

```bash
./seedflux export --url ./development.db --dialect sqlite --format sql > seed.sql
./seedflux export --url ./development.db --dialect sqlite --format csv > seed.csv
./seedflux export --url ./development.db --dialect sqlite --format jsonl > seed.jsonl
```

Export edilen row'lar aynı deterministic generation engine'den gelir; set içindeki foreign key ilişkileri korunur.

### Profiles

```bash
./seedflux generate --url ./development.db --dialect sqlite --profile demo
```

CLI override'ları profile değerlerinin üzerine yazılır:

```bash
./seedflux generate --url ./development.db --dialect sqlite --profile demo --users 500
```

## Determinism

Aynı SeedFlux version, normalized schema, configuration ve `seed` ile generated application-level values deterministic'tir.

```bash
./seedflux generate --seed 42
```

Database tarafından üretilen identity/default değerleri byte-identical garanti kapsamı dışındadır. Bu davranış kasıtlıdır; SeedFlux database'in native default mekanizmalarını gereksiz şekilde override etmez.

## Safety

SeedFlux production data copy'lamaz ve production database'lerde varsayılan olarak çalışmayı reddeder. `production`, `prod-`, `.prod` ve benzeri target sinyalleri algılanır.

Bir production target için explicit opt-in gerekir:

```bash
./seedflux generate --allow-production
```

> Bu flag yalnızca target'ın güvenli olduğundan eminseniz kullanılmalıdır. SeedFlux otomatik `TRUNCATE` veya broad `DELETE` çalıştırmaz.

## Database Desteği

| Database | Adapter | Durum |
| --- | --- | --- |
| SQLite | `modernc.org/sqlite` | Reference fixture ve end-to-end test mevcut |
| PostgreSQL | `pgx` | Driver-backed schema introspection |
| MySQL / MariaDB | `go-sql-driver/mysql` | Driver-backed schema introspection |

Detaylar için [DATABASE_SUPPORT.md](DATABASE_SUPPORT.md) dosyasına bakın.

## Test ve Development

```bash
go fmt ./...
go test ./...
go vet ./...
go build ./cmd/seedflux
```

SQLite e-commerce fixture:

```text
fixtures/sqlite/ecommerce.sql
fixtures/seedflux.yml
```

Fixture; self-reference, composite primary key, composite unique, FK, nullable alan, generated column, JSON, timestamp ve check constraint örnekleri içerir.

## Project Yapısı

```text
cmd/seedflux/          CLI entry point
internal/adapters/     PostgreSQL, MySQL ve SQLite adapter'ları
internal/schema/       Database-neutral schema model
internal/graph/        Dependency graph ve planning
internal/generator/    Deterministic semantic generators
internal/engine/       Generation, validation, manifests ve cleanup
internal/export/       SQL, CSV ve JSONL export
internal/writer/       Batched database writer
internal/safety/       Redaction ve production safeguards
fixtures/              Test schema ve configuration
docs / *.md            Architecture ve contributor documentation
```

## Katkı Sağlama

Issue, feature request ve pull request'ler memnuniyetle karşılanır. Katkı göndermeden önce test suite'i çalıştırın:

```bash
go fmt ./...
go test ./...
go vet ./...
```

Detaylar için [CONTRIBUTING.md](CONTRIBUTING.md) dosyasına bakın.

## Güvenlik

Security issue'ları public issue olarak paylaşmayın. Lütfen [SECURITY.md](SECURITY.md) içindeki reporting guidance'ı takip edin.

## License

Bu proje [MIT License](LICENSE) ile lisanslanmıştır.

---

Developed by **[muhammedkoca.com.tr](https://muhammedkoca.com.tr)** · Open-source software for reliable test data.
