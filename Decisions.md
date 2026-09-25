# Decisions

> Entries for Phases 0–4b were reconstructed on 2026-09-25 from code, in-code comments, and commit history. Dates are commit dates, or file dates for work not yet committed (Phase 3 handler/DTO, Phase 4a/4b). Where the reasoning at the time was not written down, the entry says "(reconstructed)".

## [2026-06-20] Modular monolith: one binary runs the API and the queue worker
Why: One deploy, one process to debug, and no network hops between modules. The boundaries are enforced through interfaces (repo → service → handler), so a module can be pulled out later if it ever needs to scale on its own. The email worker is a goroutine in the same process, reading RabbitMQ deliveries.
Alternative considered: microservices from day one — rejected because a single developer would pay the distributed-systems cost (deploys, tracing, network failures) before there's any load that needs it.

## [2026-06-20] Secrets only in env; non-secret tuning in a committed `config.yaml`; env overrides yaml
Why: `config.yaml` holds pool sizes, TTLs and the region, and is safe to commit. DSNs, JWT secrets and S3 keys never touch git. `.env` is only a dev convenience (`godotenv` is silent when it's missing). In prod the same keys arrive as real env vars from ECS.
Alternative considered: everything in `.env` — rejected because non-secret defaults would then be undocumented and not shared across the team.

## [2026-06-20] Fill the config struct field-by-field with `viper.GetX`, not `viper.Unmarshal`
Why: This reversed the first version. `Unmarshal` goes through mapstructure, which matches Go field names without underscores (`MaxOpenConns` → `maxopenconns`), so it never matches yaml keys like `max_open_conns`. Making it work needs a `mapstructure:` tag on every field. The explicit `GetString("database.max_open_conns")` makes each mapping visible, which suits a learning codebase. Side effect not recorded at the time: with `AutomaticEnv`, `Unmarshal` also skips keys that exist only in env (like `database.dsn`, which isn't in the yaml). Explicit `Get` calls don't have that gap.
Alternative considered: `Unmarshal` + `mapstructure` tags on every field — rejected as tag noise on every struct.

## [2026-06-21] Set explicit Postgres pool limits (25 open / 5 idle / 300s max lifetime)
Why: `database/sql` has no limit on open connections by default. Under a burst, the app can open more connections than Postgres's `max_connections` allows and take the DB down for everyone. The lifetime cap retires connections that went stale after a DB restart or network blip.
Alternative considered: driver defaults — rejected because of the unlimited open connections.

## [2026-06-23] Split a logical `File` from a physical `StorageObject`
Why: A `File` is one user's name for some bytes: owner, folder, filename, mime type. A `StorageObject` is the bytes themselves: checksum, S3 key, size, ref_count. Many `File` rows can point to one `StorageObject`, so identical content is stored once, and `ref_count` decides when the S3 object can be deleted. This split is what makes dedup (Phase 4b) and later content-defined chunking (Phase 13) possible without a schema rewrite.
Alternative considered: one `files` table with a blob/S3 key per row — rejected because every upload of the same bytes would cost a separate copy in S3, and adding dedup later would mean migrating every row.

## [2026-06-21] UUID primary keys, generated in the app (`BeforeCreate`)
Why: IDs show up in URLs (`/files/:id`). With sequential integers, anyone can guess other users' resource IDs and see how many rows exist. Generating the ID in Go means the service knows it before the INSERT. (reconstructed)
Alternative considered: `bigserial` — rejected because the IDs can be enumerated; DB-side `gen_random_uuid()` — rejected because GORM wouldn't know the ID until the insert returns (reconstructed).

## [2026-06-24] Nullable `*uuid.UUID` for parent folder, with SQL `NULL` meaning root
Why: A plain `uuid.UUID` can't express "no parent". Its zero value is `uuid.Nil` (`00000000-…`), which looks like a real ID and would need magic-value checks everywhere. A pointer maps directly to SQL `NULL`, and the repo turns `nil` into `IS NULL`.
Alternative considered: `uuid.Nil` as a "root" sentinel — rejected because of the magic value. A real per-user root-folder row — not chosen because it's an extra row to create on signup and to protect from deletion (reconstructed).

## [2026-06-24] Repositories return their own `ErrNotFound` sentinel, not `gorm.ErrRecordNotFound`
Why: If services checked `gorm.ErrRecordNotFound`, every service would depend on GORM. Swapping to pgx/sqlx later would mean touching business logic. Each repo translates the error at the boundary, so services see only `repositories.ErrNotFound`. Other errors pass through unchanged because they are real failures, not a missing row.
Alternative considered: return GORM errors directly — rejected because it couples the service layer to the ORM.

## [2026-06-24] Counters use atomic SQL expressions via `UpdateColumn`
Why: `used_storage` and `ref_count` change with `gorm.Expr("col + ?")`, which is one statement, so concurrent requests can't lose each other's updates. The code uses `UpdateColumn` rather than `Update` so that changing a counter doesn't fire hooks or bump `updated_at`.
Alternative considered: read the row, change the value in Go, `Save` — rejected because it's two round trips and two concurrent requests would overwrite each other.

## [2026-06-29] Tenant scoping is inside the repo query (`WHERE id = ? AND user_id = ?`)
Why: `FindByID`, `ListBy*`, and `Delete` on files and folders require `userID` as a parameter. A service can't forget the ownership check, because it can't call the method without one. Another user's resource returns `ErrNotFound`, the same as one that doesn't exist, so no existence information leaks. (reconstructed)
Alternative considered: fetch by ID, then compare `UserID` in the service — rejected because each call site has to remember to compare. One missed check is an IDOR bug, and a 403 confirms the resource exists (reconstructed).

## [2026-06-25] Access token is a 15-min HS256 JWT; refresh token is an opaque random string stored in Redis
Why: The access token is checked on every request without a DB/Redis lookup, and 15 minutes limits the damage if one leaks. The refresh token is 32 random bytes with no payload. It's valid only while its Redis key exists, so logout and rotation can revoke it immediately. A JWT can't be revoked before it expires.
Alternative considered: refresh token as a long-lived signed JWT — rejected because it can't be revoked without a denylist, and a denylist brings back the lookup that stateless tokens were supposed to avoid. (The config still requires `JWT_REFRESH_SECRET`, which nothing reads — see retro.)

## [2026-06-25] Refresh tokens are stored as `sha256(token)`, not bcrypt and not plaintext
Why: If Redis leaks (a dump, or someone with RedisInsight access), the attacker should get nothing they can use. SHA-256 is enough here because the token is 256 bits of randomness, so there's nothing to brute-force. The hash also has to be deterministic, because the key is looked up by it (`refresh_token:{sha256}`). (reconstructed)
Alternative considered: bcrypt — rejected because it's salted, so the token can't be looked up by key, and it's deliberately slow, which only matters for low-entropy passwords (reconstructed). Plaintext — rejected because a Redis dump would expose live sessions.

## [2026-06-25] Access-token validation pins the signing algorithm to HMAC
Why: `ValidateAccessToken` rejects any token whose `alg` isn't HMAC before handing over the key. That blocks the classic alg-confusion attacks (`alg: none`, or RS/HS swapping). (reconstructed)
Alternative considered: trust the token's `alg` header — rejected because the token would choose how it gets verified.

## [2026-06-27] Welcome email goes through RabbitMQ, fire-and-forget; registration never waits on it
Why: A slow or down SMTP server/queue must never make signup fail or slow down. `Publish` logs errors and returns nothing, so callers can't depend on it. The consumer uses `Prefetch(1)`, and it Nacks malformed payloads without requeue so a poison message can't loop forever.
Alternative considered: send SMTP inline in the request — rejected because of the added latency and because it couples signup to the mail provider being up. Transactional outbox — not done; it's the stronger guarantee, but a lost welcome email is an acceptable loss.

## [2026-07-22] Refresh fails closed: any Redis error on `/refresh` returns 401
Why: For an auth check, "can't verify" has to mean "deny". Rate limiting (Phase 7) will deliberately fail open instead, so the Redis failure policy is chosen per use case, not globally.
Alternative considered: fall back to allowing the refresh — rejected because a Redis outage would then disable token revocation.

## [2026-07-22] OAuth: CSRF state in Redis with a 10-min TTL; a Google login is linked to any existing account with the same email
Why: Redis TTL gives single-use, self-expiring state with no cleanup job. Linking by email means one person gets one account whether they signed up with a password or with Google. (reconstructed)
Alternative considered: a separate account per provider — rejected as duplicate accounts. Linking has a pre-account-takeover risk, recorded in the retro.

## [2026-07-23] Check parent-folder ownership in the service, not with a DB foreign key
Why: A foreign key only proves the parent folder exists, not that it belongs to the caller. `folders.FindByID(parentID, userID)` before the insert proves both, so a user can't attach a folder under someone else's tree by guessing its UUID. (reconstructed)
Alternative considered: an FK constraint on `parent_folder_id` — it doesn't check ownership, so it would still need the service check.

## [2026-07-24] File bytes go straight between client and S3 via presigned URLs; the API only handles metadata
Why: Bytes never pass through the Go process, so memory, bandwidth and the 15s server write timeout don't depend on file size, and the API scales by request count rather than GB. Upload is two steps: `init-upload` returns a presigned PUT, then `confirm` writes the DB rows.
Alternative considered: stream uploads through the API — rejected because every byte would cost the server twice (in and out), and large files would hit the timeouts. Cost of this choice: the server never sees the bytes, so it can't verify what was uploaded (see retro).

## [2026-07-24] One AWS SDK for both MinIO (dev) and S3 (prod), switched by endpoint config
Why: The same code path runs in dev and prod. A non-empty `S3_ENDPOINT` turns on path-style addressing for MinIO. `ensureBucket` creates the bucket at startup so dev needs no manual setup.
Alternative considered: `minio-go` in dev — rejected because two clients would mean the dev path isn't the prod path.

## [2026-07-24] Dedup by client-supplied SHA-256 before upload; S3 key is `objects/{checksum}`
Why: The client hashes the file locally. If `init-upload` finds that checksum already stored, the client skips uploading altogether — the biggest bandwidth saving dedup can give. The content-addressed key means identical bytes always go to the same object.
Alternative considered: the server hashes after upload — rejected at the time because the client would always upload in full, which removes most of the dedup benefit. This choice is the root of the confused-deputy and quota bugs in the retro; Phase 4e revisits it.

## [2026-07-24] Reference counting to decide when to delete an S3 object
Why: `ref_count` is kept on the `StorageObject` row. When a delete brings it to zero, the S3 object and the row are removed right away, so storage is freed as it's released and there's no background job.
Alternative considered: periodic mark-and-sweep of unreferenced objects — rejected because it needs a scheduler and a full scan. Its advantage, which we gave up: it tolerates crashes and races. Pure refcounting has to be exactly right under concurrency, and the current decrement isn't (Phase 4e).
