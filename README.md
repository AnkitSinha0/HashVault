<div align="center">

<img src="assets/hero.svg" width="100%" alt="HashVault: two users upload the same photo, both hash to the same SHA-256, and the bytes are stored once with ref_count 2"/>

<br/>

<a href="https://go.dev"><img alt="Go 1.26" src="https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white"/></a>
<img alt="PostgreSQL 16" src="https://img.shields.io/badge/PostgreSQL-16-4169E1?style=for-the-badge&logo=postgresql&logoColor=white"/>
<img alt="Redis 7" src="https://img.shields.io/badge/Redis-7-DC382D?style=for-the-badge&logo=redis&logoColor=white"/>
<img alt="RabbitMQ 3.13" src="https://img.shields.io/badge/RabbitMQ-3.13-FF6600?style=for-the-badge&logo=rabbitmq&logoColor=white"/>
<img alt="S3 / MinIO" src="https://img.shields.io/badge/S3-MinIO-C72E49?style=for-the-badge&logo=minio&logoColor=white"/>
<br/>
<img alt="Status: phase 4e, hardening" src="https://img.shields.io/badge/status-phase_4e_·_hardening-f59e0b?style=flat-square"/>
<img alt="Architecture: modular monolith" src="https://img.shields.io/badge/architecture-modular_monolith-6366f1?style=flat-square"/>
<img alt="Upload path: presigned URLs" src="https://img.shields.io/badge/uploads-presigned_URLs-10b981?style=flat-square"/>

<h3>A Dropbox-style storage backend where identical bytes are stored once.</h3>

<p>
<a href="#architecture">Architecture</a> ·
<a href="#how-it-works">How it works</a> ·
<a href="#quickstart">Quickstart</a> ·
<a href="#api">API</a> ·
<a href="#engineering-decisions">Decisions</a> ·
<a href="#status--roadmap">Roadmap</a>
</p>

</div>

---

## What is HashVault?

HashVault is the backend of a cloud drive: accounts, folders, files, uploads and downloads. It's written in Go as a **modular monolith**, a single binary with hard internal boundaries, and it's designed around two ideas:

1. **The API never touches file bytes.** Clients upload and download directly to S3 (or MinIO) with short-lived **presigned URLs**. The Go server only handles metadata, permissions and quota, so a 4 GB upload costs it one small JSON request.
2. **Storage is content-addressed.** A *file* is one user's name for some bytes. A *storage object* is the bytes themselves, keyed by their SHA-256 (`objects/{sha256}`). When two people upload the same photo, there are two `files` rows, one `storage_objects` row with `ref_count = 2`, and one copy in S3. Deleting a file decrements the count, and the bytes go away only when nobody references them.

Around that core are the parts a real service needs: JWT auth with rotating refresh tokens, Google sign-in, tenant-scoped queries, per-user quotas, async email over RabbitMQ, and graceful shutdown.

> [!NOTE]
> This is a learning-in-public portfolio project, built phase by phase. Every non-obvious choice is written down in [`Decisions.md`](Decisions.md), and every phase has an honest retro in [`retro.md`](retro.md), including the bugs. The [known issues](#status--roadmap) section below is deliberately not hidden.

<table>
<tr>
<td width="50%" valign="top">

**Content-addressed dedup**<br/>
SHA-256 as identity, a `UNIQUE` index on the checksum, and reference counting decide when bytes can be deleted.

</td>
<td width="50%" valign="top">

**Zero-copy uploads**<br/>
Presigned S3 `PUT`/`GET` URLs (15 min). Bytes go client ↔ object storage; the API stays on the control plane.

</td>
</tr>
<tr>
<td valign="top">

**Token design**<br/>
15-min HS256 access JWT, checked with no lookup. The opaque refresh token is stored only as `sha256(token)` in Redis and rotated on every use.

</td>
<td valign="top">

**Tenant isolation in the query**<br/>
Repository methods require a `userID` and filter `AND user_id = ?`. Another user's resource is a `404`, not a `403`.

</td>
</tr>
<tr>
<td valign="top">

**Async by default**<br/>
Signup publishes `email.welcome` to a durable topic exchange. An in-process worker consumes it with manual ack and prefetch 1.

</td>
<td valign="top">

**Production hygiene**<br/>
Server timeouts, 30 s graceful drain, fail-fast config validation, structured Zap logs, env-over-yaml config.

</td>
</tr>
</table>

---

## Architecture

<div align="center">
<img src="assets/architecture.svg" width="100%" alt="HashVault architecture: a client calls a single Go binary over HTTPS; the binary uses PostgreSQL, Redis, RabbitMQ and S3; file bytes move directly between the client and S3 via presigned URLs"/>
</div>

The binary is layered, and each layer talks to the one below only through an interface:

<div align="center">
<img src="assets/layers.svg" width="100%" alt="Request path through the binary: HTTP (Gin, middleware), handlers, services and adapters, each reached through an interface, then Postgres, S3 and RabbitMQ; the response returns the same way"/>
</div>

| Layer | Knows about | Never knows about |
|---|---|---|
| **Handlers** (`internal/handlers`) | HTTP, DTOs, the `{data}`/`{error}` envelope | SQL, Redis, S3 |
| **Services** (`internal/services`) | business rules, sentinel errors (`ErrEmailTaken`, `ErrStorageExceeded` …) | Gin, GORM types |
| **Repositories** (`internal/repositories`) | GORM, SQL | HTTP; they translate `gorm.ErrRecordNotFound` → `repositories.ErrNotFound` |
| **Adapters** (`internal/storage`, `internal/queue`, `pkg/jwt`) | one external system each | each other |

Implementations are unexported structs that their constructors return *as the interface* (services also carry a `var _ Interface = (*impl)(nil)` assertion), so an implementation that drifts from its contract is a compile error, not a runtime surprise.

---

## How it works

### Upload with deduplication

The client hashes the file *before* uploading anything, so a duplicate costs one round trip and zero bytes.

<div align="center">
<img src="assets/upload-flow.svg" width="100%" alt="Upload sequence played twice: a duplicate (init-upload answers deduplicated, nothing is uploaded) and new content (presigned PUT straight to S3), both ending in confirm, which creates the storage object or increments ref_count"/>
</div>

Download is the mirror image: `GET /files/:id/download` checks ownership, then returns a 15-minute presigned `GET` URL. On delete, the file row is soft-deleted and `ref_count` drops by one. At zero, the S3 object and its row are removed.

### Sessions: access + refresh tokens

<div align="center">
<img src="assets/session-flow.svg" width="100%" alt="Login stores sha256 of the refresh token in Redis and returns a 15-minute JWT plus an opaque refresh token; requests verify the JWT locally; refresh deletes the old token and stores a new one"/>
</div>

Refresh tokens are 256-bit random values, so storing `sha256(token)` is enough: a Redis dump yields nothing usable. If Redis is unavailable, `/refresh` **fails closed** with `401`.

### Data model

<div align="center">
<img src="assets/data-model.svg" width="100%" alt="Data model: users own folders and files, folders nest and contain files, each file references one storage object keyed by a unique SHA-256, and files can have share links; ref_count rises when a second file points at the same bytes"/>
</div>

IDs are UUIDs generated in the app (`BeforeCreate`), so they can't be enumerated and are known before the `INSERT`. `files` and `folders` are soft-deleted. `storage_objects` are immutable and hard-deleted at `ref_count = 0`.

---

## Quickstart

**Requirements:** Go 1.26+, Docker (with Compose), and `jq` for the walkthrough.

```bash
git clone https://github.com/AnkitSinha0/HashVault.git && cd HashVault
cp .env.example .env                 # local defaults: Postgres, Redis, RabbitMQ, MinIO
docker compose up -d                 # postgres · redis · rabbitmq · minio (+ redisinsight on :5540)
go run ./cmd/api                     # or `make run` for live reload with air
```

The API listens on `:8080`. `GET /health` → `{"service":"hashvault","status":"ok"}`. The MinIO console is at <http://localhost:9001> (`hashvault` / `hashvault123`), and the bucket is created on first boot.

### Walkthrough: upload, dedup, download

```bash
API=http://localhost:8080/api/v1
json='Content-Type: application/json'

# 1. sign up → access token
TOKEN=$(curl -s -X POST $API/auth/register -H "$json" \
  -d '{"name":"Asha","email":"asha@example.com","password":"correct-horse"}' | jq -r .data.access_token)

# 2. hash locally, ask for an upload URL
echo 'hello hashvault' > hello.txt
META=$(jq -n --arg sum "$(sha256sum hello.txt | cut -d' ' -f1)" --argjson size "$(wc -c < hello.txt)" \
  '{file_name:"hello.txt", mime_type:"text/plain", size:$size, checksum:$sum}')
URL=$(curl -s -X POST $API/files/init-upload -H "$json" -H "Authorization: Bearer $TOKEN" -d "$META" | jq -r .data.upload_url)

# 3. upload straight to MinIO (the API never sees the bytes), then confirm
curl -s -X PUT --upload-file hello.txt "$URL"
FILE=$(curl -s -X POST $API/files/confirm -H "$json" -H "Authorization: Bearer $TOKEN" -d "$META" | jq -r .data.id)

# 4. download through a presigned GET
curl -s "$(curl -s $API/files/$FILE/download -H "Authorization: Bearer $TOKEN" | jq -r .data.download_url)"
# → hello hashvault
```

Register a second user and run step 2 with the same file: `init-upload` answers `{"deduplicated": true}` and returns no URL. After confirming, `storage_objects` has **one** row with `ref_count = 2`:

```bash
docker compose exec postgres psql -U hashvault -d hashvault \
  -c "select left(checksum,12) as checksum, size, ref_count from storage_objects"
```

<sub>macOS: use `shasum -a 256` instead of `sha256sum`.</sub>

---

## API

All routes are under `/api/v1`. Success bodies are `{"data": …}` and errors are `{"error": "…"}`. **Auth: yes** means the route requires `Authorization: Bearer <access_token>`.

| Method | Route | Auth | Purpose | Success |
|---|---|:-:|---|---|
| `POST` | `/auth/register` | | create account, returns a token pair | `201` |
| `POST` | `/auth/login` | | email + password → token pair | `200` |
| `POST` | `/auth/refresh` | | rotate the refresh token, new pair | `200` |
| `POST` | `/auth/logout` | | revoke a refresh token | `200` |
| `GET` | `/auth/google` | | Google consent URL + CSRF `state` | `200` |
| `GET` | `/auth/google/callback` | | exchange the code → token pair | `200` |
| `POST` | `/folders` | yes | create (optionally under `parent_folder_id`) | `201` |
| `GET` | `/folders` | yes | list root folders | `200` |
| `GET` | `/folders/:id` | yes | list a folder's children | `200` |
| `PATCH` | `/folders/:id` | yes | rename | `200` |
| `DELETE` | `/folders/:id` | yes | soft-delete | `204` |
| `POST` | `/files/init-upload` | yes | quota + dedup check → presigned `PUT` (or `deduplicated: true`) | `200` |
| `POST` | `/files/confirm` | yes | record the file, link or create the storage object | `201` |
| `GET` | `/files/:id/download` | yes | presigned `GET` URL | `200` |
| `DELETE` | `/files/:id` | yes | soft-delete, `ref_count − 1`, purge at 0 | `204` |
| `GET` | `/health` | | liveness | `200` |

Error mapping is centralised in `respondServiceError`: email taken → `409`, bad credentials/token → `401`, quota exceeded → `422`, not found *or not yours* → `404`, validation → `400`.

---

## Configuration

Non-secret settings live in [`config.yaml`](config.yaml). Secrets come from the environment (`.env` is loaded for local dev). **An environment variable always wins over yaml**: `SECTION_KEY` maps to `section.key`.

| Variable | Purpose | Default (local) |
|---|---|---|
| `DATABASE_DSN` | Postgres connection string | `postgres://hashvault:hashvault@localhost:5432/hashvault?sslmode=disable` |
| `REDIS_ADDR` / `REDIS_PASSWORD` | Redis | `localhost:6379` |
| `JWT_ACCESS_SECRET` | HMAC key for access tokens (**required**) | change it! |
| `RABBITMQ_URL` | AMQP URL incl. credentials | `amqp://hashvault:hashvault@localhost:5672/` |
| `S3_ENDPOINT` | MinIO URL; leave empty for AWS S3 | `http://localhost:9000` |
| `S3_ACCESS_KEY_ID` / `S3_SECRET_ACCESS_KEY` | object storage credentials | `hashvault` / `hashvault123` |
| `OAUTH_GOOGLE_CLIENT_ID` / `_SECRET` | Google sign-in (optional) | – |
| `SERVER_PORT`, `SERVER_ENV` | listen port; `production` = JSON logs + Gin release mode | `8080`, `development` |

TTLs (`jwt.access_ttl` = 15 min, `jwt.refresh_ttl` = 7 days), pool sizes and the bucket name are in `config.yaml`.

---

## Project layout

```
cmd/api/main.go            wiring: config → infra → repos → services → routes → server; graceful shutdown
internal/
  config/                  Viper + godotenv, field-by-field load, fail-fast validation
  database/                Postgres (GORM, pool tuning, AutoMigrate) · Redis client
  models/                  User · Folder · File · StorageObject · ShareLink
  repositories/            one interface + GORM impl per aggregate · ErrNotFound
  services/                auth · oauth · folder · file (dedup, quota, ref counting)
  handlers/                thin HTTP handlers + the shared response envelope
  middleware/              RequireAuth (Bearer JWT → user id in context)
  storage/                 ObjectStore interface · S3/MinIO adapter (presign, delete)
  queue/                   RabbitMQ connection · publisher · email worker
  dto/                     request/response shapes with validation tags
pkg/
  jwt/                     token manager (HS256 access, random refresh)
  logger/                  Zap (console in dev, JSON in prod)
```

---

## Engineering decisions

The full log, with alternatives and *"would flip if"* conditions, is in [`Decisions.md`](Decisions.md). The ones that shaped the system:

| Decision | Over | Because |
|---|---|---|
| Separate `File` (name, owner) from `StorageObject` (bytes) | one `files` table with an S3 key per row | dedup became ~40 lines instead of a data migration |
| Presigned URLs for all file bytes | streaming uploads through the API | the API's bandwidth, memory and timeouts stay independent of file size |
| Modular monolith | microservices | one deployable, real boundaries, extractable later. Distribution isn't needed yet |
| Opaque refresh token, hashed in Redis | long-lived refresh JWT | revocable instantly; a leaked Redis dump is useless |
| Ownership inside the query (`AND user_id = ?`) | fetch, then compare in the service | a forgotten check can't compile into an IDOR, and existence doesn't leak |
| Counters as SQL expressions (`ref_count + 1`) | read-modify-write in Go | no lost updates under concurrency |
| Fire-and-forget email over RabbitMQ | sending SMTP inline | signup latency and availability don't depend on the mail provider |

---

## Status & roadmap

| Phase | Scope | Status |
|---|---|:-:|
| 0 | Foundation: config, logging, Postgres/Redis, graceful shutdown | done |
| 1 | Models & repositories | done |
| 2 | Auth: JWT, refresh rotation, Google OAuth, RabbitMQ worker | done |
| 3 | Folders | done |
| 4a | File upload/download via presigned URLs | done |
| 4b | Content-addressed deduplication + reference counting | done |
| **4e** | **Integration test suite (testcontainers) + concurrency and dedup security fixes** | **next** |
| 4c / 4d | Multipart & resumable uploads · adaptive chunk sizing | planned |
| 5 | Sharing (links, permissions) | planned |
| 6 | Email worker: SMTP, retries, dead-letter queue | planned |
| 7 | Rate limiting (Redis sliding window) | planned |
| 9 · 10 · 12 | Observability · CI/CD · versioned migrations | planned |
| 11 | AWS (ECS Fargate) with Terraform | planned |
| 13 | Content-defined chunking (Rabin fingerprints) for sub-file dedup | planned |

### Known issues (being fixed in phase 4e)

Found through code review and reproduced against the running system. Details, with real output, are in [`retro.md`](retro.md).

- **Dedup trusts the client's hash.** Anyone who knows a file's SHA-256 can confirm it and download bytes they never had. Confirm also doesn't verify that the upload happened: a confirm with no `PUT` succeeds. The fix: upload to a per-user staging key, have the server verify size and hash, then promote to `objects/{sha256}`.
- **Check-then-act races.** Two first-time confirms of the same checksum both try to `INSERT`. `DecrementRefCount` runs `UPDATE` then `SELECT`, so two concurrent deletes can both see zero. The quota check is read-then-write. The fixes: `INSERT … ON CONFLICT DO UPDATE … RETURNING`, `UPDATE … RETURNING ref_count`, a conditional quota `UPDATE`, and one transaction per confirm.
- **Refresh rotation is `GET` + `DEL`.** Concurrent refreshes with one token can each succeed. The fix is `GETDEL`.
- **No automated tests yet.** Phase 4e adds integration tests against real Postgres and Redis (testcontainers), starting with the concurrency cases above.

---

<div align="center">
<sub>Built by <a href="https://github.com/AnkitSinha0">Ankit Sinha</a>, one phase at a time.</sub>
</div>
