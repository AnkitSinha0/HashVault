# Retro

> Phases 0–4b written retroactively on 2026-09-25 from a code read, not at the time. `go build ./...` and `go vet ./...` pass clean. There are no tests, so every "bug" below was found by reading the code, and none has been reproduced by a test yet. Phase 4e exists to do that.
>
> **Update 2026-10-21.** A fault audit on 2026-10-19 found 32 problems (IDs **A1–A32**), 25 of them reproduced against a running stack with a probe script. Each one is recorded under the phase that introduced it, marked ✅ fixed (with how, and the evidence after the fix) or ⏳ open. The full list is in the [fault audit index](#fault-audit-index-2026-10-19) at the end. "Fixed" means fixed and re-verified in the working tree on 2026-10-21. There are still no Go tests: the evidence comes from the probe script.

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

**Fault audit 2026-10-19 — findings from this phase**
- ⏳ **A11 — The dev Compose stack is reachable from the network.** Every service is published on `0.0.0.0` with default credentials, Redis has no password, and RedisInsight has no login. Reproduced: an attacker who can reach port 6379 writes `refresh_token:{sha256(x)}` → `<victim user id>`, then calls `/auth/refresh` with `x` and gets `200` with the victim's tokens (account takeover). **Open.** Fix: `127.0.0.1:` host bindings on every port, plus `requirepass` on Redis. No phase planned.
- ⏳ **A25 — Config that does nothing, or accepts unsafe values.** `S3.UseSSL` and `JWT_REFRESH_SECRET` are read but never used, and the `.env.example` placeholders (`change-me-access-secret`, MinIO's `hashvault123`) are accepted at startup. **Open.** Fix: delete the unused keys, and refuse placeholder or short secrets in `validate`.
- ⏳ **A27 — No request body size limit, and Gin trusts every proxy.** A huge JSON body is read into memory, and `c.ClientIP()` can be spoofed with `X-Forwarded-For`, which matters once Phase 7 rate-limits by IP. **Open.** Fix: `http.MaxBytesReader` middleware and `r.SetTrustedProxies(...)`. Phase 7.
- ⏳ **A29 — In development, GORM logs every SQL statement with its values,** including the bcrypt hash in `INSERT INTO "users"`. **Open.** Fix: log at `Warn`, or with parameterized queries (`ParameterizedQueries: true`).
- ⏳ **A30 — Images aren't pinned.** `minio/minio:latest` can no longer be pulled (it only works from the local cache), and `redis/redisinsight:latest` floats. **Open.** Fix: pin each image to a version tag or digest.

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
  **✅ Fixed 2026-10-21 (A15):** one statement, `UPDATE storage_objects SET ref_count = ref_count - 1 WHERE id = ? RETURNING ref_count`, run inside the delete transaction. Its row lock serializes concurrent deletes. Evidence: 10 owners deleting their 10 references at once → `{"204":10}`, and the object was purged exactly once (0 rows left). The Phase 1 one-connection test wasn't re-run.
- `IncrementStorage` is atomic but has no limit check, so the quota check is check-then-act → race (Phase 4e).
  **✅ Fixed 2026-10-21 (A7):** confirm now reserves quota with the new `ReserveStorage`: `UPDATE users SET used_storage = used_storage + ? WHERE id = ? AND used_storage + ? <= storage_limit`, where 0 rows affected means `ErrStorageExceeded`. `IncrementStorage` remains only for refunds on delete.
- Folder delete soft-deletes only that one row. Its children keep a `parent_folder_id` that points at a deleted folder. They can't be reached from root but still count toward `used_storage`. Not on the phase plan.
- `email` has a unique index together with soft delete → a deleted user's email stays taken forever. Latent: there's no user-delete endpoint yet.
- `ShareLink` doesn't match CLAUDE.md: no permissions field, no owner column, repo methods not scoped by user, and the token is stored in plaintext (refresh tokens are hashed). Revisit before Phase 5.
- `folderRepo.Update` uses `Save`, which writes every column including `user_id`. It's safe only while the service always loads the folder first.

**Would change**
- Write the concurrency test next to each "atomic" repo method when it's written.
- Decide what cascades on delete during model design.
- Use a composite index `(user_id, parent_folder_id)` on folders, matching what `files` already has.

**Decisions that mattered most:** File/StorageObject split; tenant scoping in repo signatures.

**Fault audit 2026-10-19 — findings from this phase**
- ✅ **A15 — `DecrementRefCount` race.** Fixed; see the note on the first bullet above.
- ✅ **A28 — `IncrementRefCount` ignored `RowsAffected`.** On a row that had just been deleted, it silently updated nothing, and confirm went on to link a file to it. **Fixed 2026-10-21:** the method is gone. Confirm uses `Upsert` (`INSERT … ON CONFLICT (checksum) DO UPDATE SET ref_count = ref_count + 1 RETURNING …, (xmax = 0) AS inserted`), which always returns the row it touched and says whether it created it.
- ✅ **Repositories couldn't join a transaction** (listed in JIT Phase1 Part 4). **Fixed 2026-10-21:** every repository method now takes its handle from `dbFromContext(ctx, r.db)`, so it joins the transaction `TxManager.WithinTx` put in the context. Confirm and delete each run in one transaction.
- ⏳ **A31 — The share-link repository isn't scoped by user** (see the `ShareLink` bullet above). **Open.** Fix before Phase 5 uses it.

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

**Reproduced 2026-10-15** (snapshot `3476121`, real output in [JIT/Phase2.md](JIT/Phase2.md)):
- The GET/DEL refresh race: one token accepted up to 5 times out of 10 concurrent requests (8 of 150 trials). `GETDEL` → 0 of 150.
- The worker and publisher don't recover from a broker restart: 0 consumers, every later welcome email lost, `/health` ok.
- Timing enumeration: unknown email ≈4 ms, known email + wrong password ≈110 ms.

**Found while writing the JIT (2026-10-15), not in the list above:**
- Logout fails open: `200 logged out` during a Redis outage, and the token still refreshes afterwards.
- Concurrent signups for one email: 1 × 201 and 9 × 500 (the unique index holds, `23505` isn't mapped to `ErrEmailTaken`).
- Passwords over 72 bytes → 500 (bcrypt's `ErrPasswordTooLong`); mixed-case emails make separate accounts.
- No 500 is ever logged: `respondServiceError`'s `default` swallows the error, and there's no request logger.
- Bindings are two exact keys, but CLAUDE.md says `email.*`, so a new `email.*` event is dropped unrouted. RabbitMQ down at boot is fatal.
- `fetchGoogleProfile` ignores the HTTP status → a non-200 can create a user with `email = ""` (code read).
- `oauth_service.go` lacks the `var _` interface check; `GoogleRedirect` also calls `c.JSON` directly; `var _ = time.Second` keeps an unused import alive.
- The tree didn't build from `48dd729` (06-29) to `1ceae57` (07-22): wiring was committed before what it wired.

**Predictions I got wrong:** none recorded. Phase 2 predates the JIT spec, so there's no `Phase2.predict.md`.

**Fault audit 2026-10-19 — findings from this phase** (all still open; details in the bullets above and in JIT/Phase2.md Part 4)
- ⏳ **A9 — Refresh rotation is `GET` then `DEL`,** so one token can be accepted several times (up to 5 of 10 concurrent requests). Fix: `GETDEL`, verified with a throwaway build on 2026-10-15 (0 of 150 trials). Phase 4e test #6.
- ⏳ **A12 — Google sign-in links to any account with the same email,** without checking Google's `verified_email` (pre-account takeover). Code read. Fix: require `verified_email`, and link to an existing password account only after a login to it.
- ⏳ **A13 — OAuth `state` isn't bound to the browser** (login CSRF). Code read. Fix: an HttpOnly, SameSite=Lax state cookie, compared on callback.
- ⏳ **A19 — Logout fails open:** `200 logged out` during a Redis outage, and the token still refreshes afterwards. Fix: return the `DEL` error (503).
- ⏳ **A20 — Account edge cases:** login timing reveals which emails exist (≈4 ms vs ≈110 ms); concurrent signups → 9 × 500 instead of 409; passwords over 72 bytes → 500; emails are case-sensitive. Fixes: a dummy bcrypt compare on unknown emails; map `23505` on `idx_users_email` to `ErrEmailTaken`; `max=72` bytes; lowercase emails plus a unique index on `lower(email)`.
- ⏳ **A21 — The email worker and publisher never reconnect after a broker restart,** and `/health` stays ok. Fix: watch `NotifyClose`, redial with backoff, report the broker in a readiness check. Phase 6.
- ⏳ **A22 — `fetchGoogleProfile` ignores the HTTP status,** so a non-200 can create a user with `email = ""`. Code read. Fix: check the status, and reject an empty email.
- ⏳ **A26 — 500s are never logged,** and there's no request log (`respondServiceError`'s `default` drops the error). Fix: log in the `default` branch, plus request-logging middleware with a request ID. Phase 9.

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

**Fault audit 2026-10-19 — findings from this phase**
- ⏳ **A18 — Deleting a folder doesn't touch its contents.** Reproduced: after `DELETE /folders/:id` → `204`, a file inside it was still downloadable, its subfolder was still readable by ID, and `used_storage` was unchanged. **Open.** Fix: either refuse to delete a non-empty folder, or soft-delete the subtree (recursive CTE) and its files in one transaction, refunding their quota.
- ⏳ **A23 — There's no endpoint that lists files** (third bullet above). **Open.** Fix: `GET /folders/:id/files` and `GET /files?folder_id=` on top of `ListByFolder`, with pagination.

---

## Phase 4a — Basic file upload via presigned URLs (≈2026-07-24)

**Intended:** init → presigned PUT → confirm; presigned GET download; delete. The API stays on the control plane.

**Went well**
- Bytes never pass through Go. The 15s server timeouts are fine at any file size.
- One SDK covers MinIO and S3; `ensureBucket` means dev needs no setup.

**Went wrong**
- **`confirm` never checks S3.** No `HeadObject`: a client can confirm a file it never uploaded, and the server records a `StorageObject` for bytes that don't exist.
  **✅ Fixed 2026-10-21 (A4):** confirm calls `StatObject` (a `HEAD` with checksum mode) on the caller's staging key. No object → `422 upload missing or does not match its checksum`. Evidence: confirm with no PUT → `422` (was `201`).
- **The size is whatever the client says.** The presigned PUT isn't bound to a Content-Length, and confirm trusts `req.Size`. Claim 1 byte, upload 5 GB.
  **✅ Fixed 2026-10-21 (A5):** the presigned PUT now signs `Content-Length`, confirm compares the size the store recorded, and quota is charged with the object's real size. Evidence: a 1 MiB PUT to a URL signed for 1 byte → `403 SignatureDoesNotMatch`; confirm → `422`. Before: `201`, charged 1 byte.
- Delete runs four steps with no transaction (file row → decrement → S3 delete → row delete). If a later step fails, earlier ones aren't rolled back. Deleting from S3 before the DB row means a failed row delete leaves a `StorageObject` whose bytes are gone, and the next dedup hit links new users to nothing.
  **✅ Fixed 2026-10-21 (A3).** The audit showed this wasn't hypothetical: it failed on *every* last-reference delete. See A3 below.

**Surprised us:** the presigned design was chosen for scale, but its real cost is trust: once the server stops seeing the bytes, every property of the bytes (existence, size, hash) is only something the client claims.

**Would change:** `HeadObject` on confirm and compare the size (and ideally the checksum, via S3's `x-amz-checksum-sha256`).

**Decisions that mattered most:** presigned URLs.

**Fault audit 2026-10-19 — findings from this phase**
- ✅ **A3 — Deleting the last copy of a file broke that content for everyone.** The old order was: soft-delete the file row → delete the S3 bytes → delete the `storage_objects` row. The last step always failed with `violates foreign key constraint "fk_files_storage_object" (SQLSTATE 23503)`, because the *soft-deleted* file row still references the object. Result: a 500, the bytes gone, the quota not refunded, and a dead `ref_count 0` row left behind. Every later upload of the same bytes was linked to that dead row (even a fully verified upload, since the upsert found the row and skipped the copy), and their downloads failed with `NoSuchKey`.
  **Fixed 2026-10-21:** `Delete` is one transaction: scoped soft delete → refund `StorageObject.Size` → `DecrementRefCount … RETURNING` (locks the row). At 0: `PurgeTombstones` hard-deletes the soft-deleted file rows that still reference the object → delete the object row → delete the S3 bytes **last**. If any step fails, everything rolls back, and the bytes are only removed when nothing else can fail after them. Evidence: last-reference delete → `204` (was `500`); no object row and no file row left; `used_storage` 23 → 0 (was stuck at 23); a later upload of the same bytes downloads correctly (was `NoSuchKey`); 10 concurrent deletes → `{"204":10}`; deleting again → `404`.
- ✅ **A4, A5 — Confirm without an upload, and client-claimed size.** Fixed; see the notes on the bullets above.
- ✅ **A7 — The quota check was check-then-act.** Reproduced: 10 concurrent 50-byte confirms against a 100-byte limit → `300 / 100` used. **Fixed 2026-10-21:** `ReserveStorage` is a conditional `UPDATE` inside the confirm transaction. Evidence: `{"201":2,"422":8}`, `100 / 100`.
- ✅ **A8 — A size near `int64` max overflowed the quota check.** `used + size` wrapped negative, passed, then confirm failed with `bigint out of range` after creating the file row (uncharged). **Fixed 2026-10-21:** `size` is capped at 5 GiB in the DTO (`lte=5368709120`), which is also S3's single-PUT limit, and confirm runs in a transaction. Evidence: `400`.
- ✅ **A10 — The uploader controlled the stored object's Content-Type.** HTML uploaded as `pic.png` with `Content-Type: text/html` was served back as `text/html`, which is stored XSS on the storage origin. **Fixed 2026-10-21:** promotion copies the bytes with `MetadataDirective: REPLACE` and `application/octet-stream`, and the presigned GET forces `ResponseContentType: application/octet-stream` plus `Content-Disposition: attachment; filename*=UTF-8''<name>`. Evidence: the same file now downloads as `application/octet-stream`, as an attachment.
- ⏳ **A24 — Presigned URLs use the S3 endpoint the API sees** (`http://localhost:9000`, or `http://minio:9000` inside Docker), which clients can't reach in a container or production setup. **Open.** Fix: a separate public endpoint for presigning (`S3_PUBLIC_ENDPOINT`), or a CDN/domain in front of the bucket.

---

## Phase 4b — Deduplication (≈2026-07-24)

**Intended:** skip the upload when the checksum already exists; refcount shared objects; per-user quota.

**Went well**
- Content-addressed keys + File/StorageObject split → dedup came to about 40 lines of service code.

**Went wrong**
- **Confused deputy** (already in CLAUDE.md 4e): know a hash → claim the file → download bytes you never had. `init-upload` also answers "does anyone have this hash?" for any hash.
  **✅ Fixed 2026-10-21 (A2, A16):** possession-scoped dedup. See A2 below.
- **Overwrite** (already in CLAUDE.md 4e): the presigned PUT targets the shared `objects/{checksum}`, so anyone can replace stored bytes.
  **✅ Fixed 2026-10-21 (A1):** nobody gets a URL for `objects/` anymore. See A1 below.
- **Quota underflow — NEW, not in CLAUDE.md.** Confirm charges the client's `req.Size`; delete refunds the real `StorageObject.Size`. Confirm a dedup hit of a 5 GB object claiming `size: 1`, delete it → `used_storage` drops by ~5 GB. Repeat → negative usage → unlimited quota.
  **✅ Fixed 2026-10-21 (A6):** confirm charges `StorageObject.Size`, the same number delete refunds, and a mismatched size claim fails verification. Evidence: claiming 1 byte for a 1 MiB object → `422`; an honest upload charges 1,048,576 and its delete returns `used_storage` to 0 (was −1,048,575).
- The three races already listed under Phase 4e (decrement, find-then-create, check-then-act quota).
  **✅ Fixed 2026-10-21 (A15, A14, A7):** `UPDATE … RETURNING`, an `ON CONFLICT` upsert, and a conditional quota `UPDATE`. See the A-items.

**Surprised us**
- Portfolio report 001 ("Content-Addressable Storage") says both refcount operations are atomic SQL and that the 50-concurrent-confirm case "is tested against a real Postgres". The decrement is two statements, and the repo has no tests. The quota section ("accounting is per user") misses the underflow.
- `ObjectStore` interface, `tx.go`, and `test.txt` (the 4e plan) all date from 2026-08-21: the 4e refactor was started and left unfinished. `fileService` still depends on `*S3Client`, and no repo uses `dbFromContext`.

**Would change:** never trust a client-supplied number for accounting; take size from the `StorageObject` row. Write the concurrency tests before publishing claims about concurrency.

**Decisions that mattered most:** client-side hash dedup (the whole 4e security section comes from it); refcounting over GC.

**Fault audit 2026-10-19 — findings from this phase**
- ✅ **A1 — Anyone could replace a file's contents for everyone (content poisoning).** Every upload URL pointed at the shared `objects/{sha256}` key, and nothing checked the bytes against the hash. Reproduced: Mallory took a URL for `hash(real installer)`, uploaded malware and confirmed; the victim's later upload of the real installer was deduplicated, and their download returned `MALWARE payload`.
  **Fixed 2026-10-21:**
  - Uploads go to a per-user staging key, `staging/{userID}/{sha256}`.
  - The presigned PUT carries `x-amz-checksum-sha256`, so MinIO/S3 rejects bytes that don't match (`XAmzContentChecksumMismatch`).
  - That header isn't part of the URL signature, so confirm doesn't rely on it. It reads the SHA-256 the *store* recorded (`HEAD` with checksum mode): an upload sent without the header has none, and fails.
  - Only then is the staged object copied to `objects/{sha256}`, with `CopySourceIfMatch` on the ETag that was checked, so a re-upload in between can't slip through. The staged copy is then deleted.
  - A lifecycle rule expires unconfirmed staging objects after 1 day.

  Evidence: evil bytes with the header → `400 XAmzContentChecksumMismatch`; without it → stored in staging, but confirm → `422`; 0 object rows for the hash; the victim downloads the real installer.
- ✅ **A2 — Knowing a hash was enough to download the file.** Reproduced: Bob, holding only the hash of Asha's private notes, confirmed it and downloaded them. **Fixed 2026-10-21:** possession-scoped dedup. The upload may be skipped only if the caller already owns a live file with that checksum (`files.UserHasLiveCopy`). Everyone else uploads, and storage is still shared, because a verified upload of existing content just adds a reference. Evidence: Bob → `deduplicated: false`, confirm without bytes → `422`; Asha re-uploading her own file → `deduplicated: true`; Bob uploading the real bytes → `201`, still 1 object row (`ref_count 3`).
- ✅ **A6 — Quota underflow.** Fixed; see the bullet above.
- ✅ **A14 — Concurrent first confirms of new content** both `INSERT`ed, and the loser hit `idx_storage_objects_checksum` → 500 (1 of 10). **Fixed 2026-10-21:** a single `INSERT … ON CONFLICT (checksum) DO UPDATE SET ref_count = ref_count + 1 RETURNING …`. Evidence: 10 concurrent confirms → `{"201":10}`, `ref_count` 10 = 10 file rows.
- ✅ **A16 — `init-upload` told anyone whether *any* user stored a file.** **Fixed 2026-10-21** with A2: `deduplicated: true` now only reflects the caller's own files.
- ✅ **A17 — Checksums weren't normalised.** The same bytes with an uppercase hash became a second object, and the `hexadecimal` validator also accepted a `0x` prefix. **Fixed 2026-10-21:** `normalizeChecksum` lowercases and requires `^[0-9a-f]{64}$`. Evidence: uppercase → `deduplicated: true` for its owner; `0x…` → `400`.

**Introduced by the fixes (2026-10-21), accepted for now**
- Two users with the same file now each upload it once (bandwidth). Storage is still shared. That's the trade-off of possession-scoped dedup.
- **The upload API changed.** Clients must send `x-amz-checksum-sha256` (returned in `upload_headers`); a client that doesn't gets `422` at confirm.
- The S3 copy (confirm) and S3 delete (delete) run inside the DB transaction, so each holds a pooled connection for one network call.
- If the final `COMMIT` fails *after* the S3 delete succeeded, the object row comes back without its bytes. The window is tiny; a reconciling GC job would close it.
- Rows broken by the old delete code (`ref_count <= 0`) aren't repaired automatically. Clean them up once with: `DELETE FROM files WHERE deleted_at IS NOT NULL AND storage_object_id IN (SELECT id FROM storage_objects WHERE ref_count <= 0); DELETE FROM storage_objects WHERE ref_count <= 0;`

---

## Fault audit index (2026-10-19)

32 findings from a code read plus probes against a fresh stack on 2026-10-19; fixes verified 2026-10-21. "Evidence" says how each one was found: **run** = reproduced against the running system, **code** = found by reading only. Details are under each phase above.

| ID | Finding | Phase | Evidence | Status |
|---|---|---|---|---|
| A1 | Content poisoning via the shared `objects/{sha256}` key | 4b | run | ✅ staging key + store-verified SHA-256 + conditional copy |
| A2 | Hash alone gives access to another user's file | 4b | run | ✅ possession-scoped dedup |
| A3 | Deleting the last copy breaks that content for everyone | 4a | run | ✅ one transaction, tombstones purged, S3 deleted last |
| A4 | Confirm without an upload succeeds | 4a | run | ✅ `StatObject` on the staging key |
| A5 | Size is whatever the client claims | 4a | run | ✅ signed `Content-Length` + stored size |
| A6 | Quota driven negative (charge ≠ refund) | 4b | run | ✅ charge `StorageObject.Size` |
| A7 | Quota check-then-act race (300/100) | 1 / 4a | run | ✅ conditional `UPDATE` (`ReserveStorage`) |
| A8 | `int64` overflow in the quota check | 4a | run | ✅ 5 GiB cap + transaction |
| A9 | Refresh rotation `GET`+`DEL` race | 2 | run | ⏳ `GETDEL` (4e) |
| A10 | Stored XSS via uploader-controlled Content-Type | 4a | run | ✅ octet-stream + attachment |
| A11 | Dev Compose exposed (Redis no password, `0.0.0.0`) | 0 | run | ⏳ |
| A12 | OAuth links by unverified email | 2 | code | ⏳ |
| A13 | OAuth state not browser-bound | 2 | code | ⏳ |
| A14 | Concurrent first confirms → 500 | 4b | run | ✅ `ON CONFLICT` upsert |
| A15 | `DecrementRefCount` UPDATE-then-SELECT race | 1 | run | ✅ `UPDATE … RETURNING` in a transaction |
| A16 | `init-upload` existence oracle | 4b | run | ✅ (with A2) |
| A17 | Checksum case not normalised; `0x` accepted | 4b | run | ✅ `normalizeChecksum` |
| A18 | Folder delete doesn't cascade | 3 | run | ⏳ |
| A19 | Logout fails open | 2 | run | ⏳ |
| A20 | Login timing, signup race, 72-byte passwords, email case | 2 | run | ⏳ |
| A21 | Worker/publisher never reconnect | 2 | run | ⏳ (6) |
| A22 | Google profile status ignored | 2 | code | ⏳ |
| A23 | No endpoint lists files | 3 | code | ⏳ |
| A24 | Presigned URLs use the internal S3 host | 4a | code | ⏳ |
| A25 | Unused config, placeholder secrets accepted | 0 | code | ⏳ |
| A26 | 500s never logged, no request log | 2 | run | ⏳ (9) |
| A27 | No body limit; all proxies trusted | 0 | code | ⏳ (7) |
| A28 | `IncrementRefCount` ignored `RowsAffected` | 1 | code | ✅ replaced by the upsert |
| A29 | Dev SQL log prints password hashes | 0 | code | ⏳ |
| A30 | Unpinned images | 0 | code | ⏳ |
| A31 | Share-link repo not user-scoped | 1 | code | ⏳ (before 5) |
| A32 | No automated tests | — | — | ⏳ (4e) |

**Totals:** 14 fixed, 18 open.
