# Retro

> Phases 0–4b written retroactively on 2026-09-25 from a code read, not at the time. `go build ./...` and `go vet ./...` pass clean. There are no tests, so every "bug" below was found by reading the code, and none has been reproduced by a test yet. Phase 4e exists to do that.

---

## Phase 0 — Foundation (2026-06-20 → 2026-06-23)

**Intended:** config, logger, Postgres + Redis clients, Docker Compose, a `main.go` that wires everything and shuts down gracefully.

**Went well**
- The split between committed yaml and env secrets held up. No secret has ever been committed (`.env` was gitignored from the first commit).
- Graceful shutdown (SIGTERM → cancel the worker → drain HTTP for 30s) and explicit pool limits from day 1.
- Health checks on every compose service.

**Went wrong**
- First config design (`viper.Unmarshal`) was reversed within hours: mapstructure's field naming doesn't match snake_case yaml keys.
- `/health` returns `ok` without checking Postgres/Redis/RabbitMQ. It's a liveness check pretending to be a readiness check, and an ALB (Phase 11) would keep routing to a task whose DB is down.
- `log.Fatal` inside the server goroutine skips all the `defer`s in `main`.

**Surprised us**
- Commit `bfa5ee9` is titled "fixed bug of cancel not being deferred", but its diff only removes a comment. The history doesn't show what the bug or the fix was.

**Would change:** make `/health` check dependencies (or split into `/livez` + `/readyz`).

**Decisions that mattered most:** secrets/env split; manual `viper.Get` (the only reversal this phase).

---

## Phase 1 — Models & Repositories (2026-06-21 → 2026-07-01)

**Intended:** 5 models (User, Folder, File, StorageObject, ShareLink) + a repository interface/implementation for each, with services kept independent of GORM.

**Went well**
- Splitting File from StorageObject made Phase 4b dedup a feature to add, not a migration.
- Every file/folder repo method takes `userID`, so ownership checks can't be skipped by accident.
- `ErrNotFound` kept GORM out of services; that held through Phases 2–4b.
- Heavy "why" comments in `user.go`/`folder.go`. Phase 1 was a learning phase, and the comments did their job.

**Went wrong / latent bugs**
- `DecrementRefCount` runs an UPDATE and then a separate SELECT. Two concurrent deletes can both see 0 → the S3 object is purged twice. It's written as if it were atomic, but it isn't. Scheduled in Phase 4e.
- `IncrementStorage` is atomic but has no limit check, so the quota check is check-then-act → race (Phase 4e).
- Folder delete soft-deletes only that one row. Its children keep a `parent_folder_id` that points at a deleted folder. They can't be reached from root but still count toward `used_storage`. Not on the phase plan.
- `email` has a unique index together with soft delete → a deleted user's email stays taken forever. Latent: there's no user-delete endpoint yet.
- `ShareLink` doesn't match CLAUDE.md: no permissions field, no owner column, repo methods not scoped by user, and the token is stored in plaintext (refresh tokens are hashed). Revisit before Phase 5.
- `folderRepo.Update` uses `Save`, which writes every column including `user_id`. It's safe only while the service always loads the folder first.

**Would change**
- Write the concurrency test next to each "atomic" repo method when it's written.
- Decide what cascades on delete during model design.
- Use a composite index `(user_id, parent_folder_id)` on folders, matching what `files` already has.

**Decisions that mattered most:** File/StorageObject split; tenant scoping in repo signatures.

---

## Phase 2 — Auth (JWT + OAuth) + queue (2026-06-25 → 2026-07-22)

**Intended:** register/login/refresh/logout, Google OAuth, auth middleware, RabbitMQ publisher + email worker skeleton.

**Went well**
- Opaque refresh tokens, hashed in Redis. HMAC alg pinned on validate. bcrypt for passwords.
- Signup can't fail because of the email path (fire-and-forget publish).
- The worker exits cleanly on context cancel. Malformed messages are Nacked without requeue.

**Went wrong**
- **Refresh rotation isn't atomic.** `Refresh` does `GET` then `DEL` as two separate calls. Two concurrent requests with the same token both pass the GET, and both get a new token pair. Fix: `GETDEL` (one command). The `DEL` error is also ignored (the comment accepts that).
- **No reuse detection.** Once a token has been rotated, presenting it again just returns 401. The session family isn't revoked. If an attacker refreshes first, the victim is logged out and the attacker keeps the session.
- **OAuth pre-account-takeover.** The Google callback logs into any existing account with that email. Signup never verifies email ownership, and the callback doesn't check Google's `verified_email`. An attacker can register `victim@gmail.com` with a password before the victim ever signs in; when the victim later uses Google, they land in an account the attacker still has a password for.
- **OAuth state isn't bound to the browser.** State is returned in the JSON body and stored globally in Redis, not in a cookie, so any valid state works from any browser → login CSRF is still possible.
- `JWT_REFRESH_SECRET` is required at startup but read by nothing. Looks like a leftover from a JWT-refresh design.
- The worker never reconnects. If RabbitMQ drops, it logs "channel closed unexpectedly" and stops for good while the API keeps running. Publish has no publisher confirms, so "persistent" messages can still be lost in flight.
- Smaller: `Logout` calls `c.JSON` directly (breaks the handler rule); the OAuth callback repeats `issueTokenPair`; it compares with `err != ErrNotFound` instead of `errors.Is`; login without a user skips bcrypt, so timing reveals whether an email exists (and `/register`'s 409 reveals it anyway).

**Surprised us**
- Portfolio report 003 ("JWT Rotation") says a refresh token "works exactly once" and that a stolen one is "revealed the moment both parties try to use it". Neither is true in the code: the GET/DEL race allows two uses, and nothing detects reuse. The article describes the intended design, not what was built.

**Would change:** `GETDEL`; reuse detection with family revocation; bind OAuth state to a cookie; require `verified_email` before linking accounts; remove `RefreshSecret`.

**Decisions that mattered most:** opaque refresh + hashing; fail-closed refresh; linking OAuth by email (the risky one).

---

## Phase 3 — Folders (2026-06-30 → 2026-07-23)

**Intended:** create / list root / list children / rename / soft-delete, all scoped by user.

**Went well**
- Parent ownership is checked before create and before listing children, so a user can't attach folders to, or browse, someone else's tree.
- Handlers are thin and use the respond helpers; UUID parsing returns 400 before the service is called.

**Went wrong**
- Delete doesn't cascade (see Phase 1). This is where it becomes user-visible: delete a folder and its contents disappear from view but keep using quota.
- Duplicate names are allowed within one folder. No unique `(user_id, parent_folder_id, name)`.
- `GET /folders/:id` returns only subfolders. There's no endpoint that lists files (`FileRepository.ListByFolder` exists, but nothing calls it).

**Would change:** decide on the cascade (recursive CTE soft-delete vs. refusing to delete a non-empty folder) before building anything on top.

**Decisions that mattered most:** ownership check in the service rather than an FK.

---

## Phase 4a — Basic file upload via presigned URLs (≈2026-07-24)

**Intended:** init → presigned PUT → confirm; presigned GET download; delete. The API stays on the control plane.

**Went well**
- Bytes never pass through Go. The 15s server timeouts are fine at any file size.
- One SDK covers MinIO and S3; `ensureBucket` means dev needs no setup.

**Went wrong**
- **`confirm` never checks S3.** No `HeadObject`: a client can confirm a file it never uploaded, and the server records a `StorageObject` for bytes that don't exist.
- **The size is whatever the client says.** The presigned PUT isn't bound to a Content-Length, and confirm trusts `req.Size`. Claim 1 byte, upload 5 GB.
- Delete runs four steps with no transaction (file row → decrement → S3 delete → row delete). If a later step fails, earlier ones aren't rolled back. Deleting from S3 before the DB row means a failed row delete leaves a `StorageObject` whose bytes are gone, and the next dedup hit links new users to nothing.

**Surprised us:** the presigned design was chosen for scale, but its real cost is trust: once the server stops seeing the bytes, every property of the bytes (existence, size, hash) is only something the client claims.

**Would change:** `HeadObject` on confirm and compare the size (and ideally the checksum, via S3's `x-amz-checksum-sha256`).

**Decisions that mattered most:** presigned URLs.

---

## Phase 4b — Deduplication (≈2026-07-24)

**Intended:** skip the upload when the checksum already exists; refcount shared objects; per-user quota.

**Went well**
- Content-addressed keys + File/StorageObject split → dedup came to about 40 lines of service code.

**Went wrong**
- **Confused deputy** (already in CLAUDE.md 4e): know a hash → claim the file → download bytes you never had. `init-upload` also answers "does anyone have this hash?" for any hash.
- **Overwrite** (already in CLAUDE.md 4e): the presigned PUT targets the shared `objects/{checksum}`, so anyone can replace stored bytes.
- **Quota underflow — NEW, not in CLAUDE.md.** Confirm charges the client's `req.Size`; delete refunds the real `StorageObject.Size`. Confirm a dedup hit of a 5 GB object claiming `size: 1`, delete it → `used_storage` drops by ~5 GB. Repeat → negative usage → unlimited quota.
- The three races already listed under Phase 4e (decrement, find-then-create, check-then-act quota).

**Surprised us**
- Portfolio report 001 ("Content-Addressable Storage") says both refcount operations are atomic SQL and that the 50-concurrent-confirm case "is tested against a real Postgres". The decrement is two statements, and the repo has no tests. The quota section ("accounting is per user") misses the underflow.
- `ObjectStore` interface, `tx.go`, and `test.txt` (the 4e plan) all date from 2026-08-21: the 4e refactor was started and left unfinished. `fileService` still depends on `*S3Client`, and no repo uses `dbFromContext`.

**Would change:** never trust a client-supplied number for accounting; take size from the `StorageObject` row. Write the concurrency tests before publishing claims about concurrency.

**Decisions that mattered most:** client-side hash dedup (the whole 4e security section comes from it); refcounting over GC.
