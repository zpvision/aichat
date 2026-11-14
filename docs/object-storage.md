# Object storage — Stage 9A.1

The Go monolith supports local files and private S3-compatible objects using AWS SDK for Go v2 (`service/s3`, static `credentials`). FinTalent's local objectstorage implementation was inspected as a reference; no multipart/video infrastructure was copied. No SDK protocol/signing is implemented in application code. Endpoint configuration follows the [AWS SDK BaseEndpoint documentation](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-endpoints.html).

## Configuration

```
STORAGE_PROVIDER=local
S3_ENDPOINT=https://storage.yandexcloud.net
S3_REGION=ru-central1
S3_BUCKET=
S3_ACCESS_KEY=
S3_SECRET_KEY=
S3_FORCE_PATH_STYLE=true
S3_PRESIGN_TTL_SECONDS=300
```

Set provider `s3` explicitly to use remote storage for new uploads. All S3 settings except the path-style/TTL defaults are mandatory in this mode, including development. No credentials are generated, logged, returned to clients or committed. Load protected environment values before running Go; Go itself does not automatically read .env. The Windows helper reads backend/.env and now accepts digit-containing names such as S3_ACCESS_KEY. The locally supplied root .env was used only for explicitly authorized tests and was not modified.

TTL is validated (60–900s) for configuration compatibility; this release does not issue presigned URLs. S3 browser CORS is unnecessary: uploads/downloads go through Go. Keep the bucket private; ACL/policy/CDN settings are not changed. SDK redirects are rejected, retries bounded to two and each object operation has a 15-second context deadline; HeadBucket checks have a two-second deadline. SDK errors are reduced to safe unavailable/not-found errors; no HTTP/S3 request details escape the adapter.

RU uses its own regional bucket/account/DB; EU must configure an independent regional provider/account/DB. The interface does not depend on Yandex. No replication or automatic RU/EU transfer exists. Deployment egress and credentials must enforce residency.

## Registry, keys and access

`knowledge.FileStorage` remains the small shared interface: Put, PutAt, Read, Size and Delete. LocalStorage remains usable directly in unit tests. ObjectStore routes opaque UUID file references through `object_files` to the recorded provider. This permits local and S3 objects to coexist during migration. Knowledge originals and widget logos have separate purposes/roots. The product has no separate company-logo entity; the existing company-branded widget logo is covered.

New remote keys: `organizations/{organization_uuid}/{knowledge|logos}/{opaque_uuid}`. Filenames never become keys. Source UUIDs, object UUIDs and physical keys are server generated. S3 identity hashes endpoint/region/bucket, excluding credentials; mismatched deployment identity is rejected instead of reading from a different bucket. New test API installations use `S3_KEY_PREFIX=integration-tests/{unique-run-id}/`; only this development prefix format is accepted and production forbids it.

Every document route authorizes organization membership and assistant scope before resolving metadata. Public widget IDs only resolve their own logo; they cannot select arbitrary object keys or read knowledge originals. Registry purposes and optional organization scope prevent a document from being served as a logo. GET `/api/v1/organizations/{id}/assistants/{assistantId}/knowledge/{documentId}/original` requires cabinet authorization, returns an attachment with nosniff and never reveals the S3 URL. The cabinet provides a translated download link. Logo delivery remains the existing narrowly scoped public Go endpoint.

PostgreSQL retains articles, extracted text, chunks, vectors, dialogs, leads and file metadata. Only original document bytes and logo PNG bytes are stored remotely. Existing format/upload/zip/PDF limits remain in force; logo images are decoded, bounded and re-encoded as PNG. Bounded reads verify recorded SHA-256 and size. New upload reads back the bytes before marking the object ready. Object readiness and knowledge-index readiness are separate states.

## Lifecycle and quotas

Upload reserves existing billing bytes first, registers a pending object before PUT, verifies bytes, marks it ready, then creates the source and queues existing processing. An error cannot publish an index. Source publication still uses the original embedding/model/revision/lease fencing. Workers read original bytes via the registry; no lasting local staging file is required. Reindex keeps the previous active revision until successful publication.

Failed/uncertain writes retain a registered object until it can be safely removed. Existing knowledge reservation cleanup releases quota only after physical deletion. Logo database failures no longer immediately delete a possibly committed logo. Administrative orphan cleanup considers only registered objects older than one day with no source/widget reference, at most100 per batch. It never lists/deletes an arbitrary bucket prefix. Keep writers stopped for applied maintenance cleanup. No automatic bucket sweep exists.

The quota remains logical original-file bytes, not duplicate physical copies during migration/export. Moving a file only changes its registry location; billing UUID, bytes and counters do not change. Retained local migration copies/backups require a separately approved retention/removal procedure, especially after a later customer erasure request. No GDPR compliance claim follows from this adapter.

## Migration and diagnostics

Apply appended migration `010_object_storage.sql` through the normal migrator. It records existing document/logo references as local, including missing originals; it does not move/delete files or alter active RAG revisions. Existing migrations are unchanged.

The deployment-admin credential gate is the same as billing CLI; secrets belong in environment, not command arguments. From backend:

```
go run ./cmd/storage --purpose knowledge --root ./data/knowledge
go run ./cmd/storage --purpose knowledge --root ./data/knowledge --apply
go run ./cmd/storage --purpose logos --root ./data/widget --apply
```

Default action is `migrate`, default mode dry-run. Each invocation processes at most100 referenced records ordered by UUID. Resume using `--after LAST_REPORTED_UUID`; repeat full scans to recheck earlier failures. Reports contain only IDs/states. Missing/unreadable local files report `missing` and preserve metadata/index; failures report `failed` and make the command exit nonzero after the batch. No real user migration was performed during this stage.

Each copy gets its own registered pending UUID before network I/O. After read-back verification, one short PostgreSQL transaction replaces the original registry location and removes temporary copy metadata. On rollback/crash the registered copy remains discoverable for cleanup; uncertain commits never cause blind deletion. Already migrated objects are read/verified and skipped. Local originals are never deleted by migration. Coordinate writers/backup before applying maintenance.

`cmd/operations --action originals --root ...` understands the registry when migration010 exists and falls back to legacy local diagnostics otherwise. The two missing originals in the older test database remain untouched. Unknown-size originals continue to block new uploads for their company; existing ready chunks are not disabled automatically. No empty replacements or invented repairs are created.

Orphans: `cmd/storage --action cleanup-orphans --purpose knowledge` is dry-run. Add `--apply` only after reviewing the exact ID report and stopping writers; repeat for logos. This removes known unreferenced registered objects, not arbitrary cloud content.

## Backup and restore

PostgreSQL dump does NOT contain S3 bytes. Stop all writers. Export referenced, verified objects to private directories, one purpose/batch at a time:

```
go run ./cmd/storage --action export --purpose knowledge --directory PRIVATE_EXPORT/knowledge
go run ./cmd/storage --action export --purpose logos --directory PRIVATE_EXPORT/widget
```

Requires `BACKUP_WRITERS_STOPPED=yes`. Use those directories as KNOWLEDGE_FILE_ROOT/WIDGET_FILE_ROOT for the offline snapshot wrapper; when STORAGE_PROVIDER=s3 also set S3_BACKUP_EXPORT_VERIFIED=yes only after every batch succeeds. The SHA manifest covers exported files. Archive configuration/credentials separately in secure escrow, never plaintext in this snapshot. Do not automatically back up the whole bucket or place PG dumps in the working bucket.

Restore the DB into a NEW *_test database. `cmd/storage --action restore-local --directory ... --purpose ...` checks exported SHA/size against registry metadata; default dry-run, `--apply` changes restored registry entries to local. It is deliberately restricted to *_test DBs. Start restored API with local roots pointing to the verified snapshot directories. Original S3 objects are not deleted or modified. Production restore-to-a-different-S3-bucket needs a separately reviewed maintenance procedure; it is not automated here.

## Verification

S3 unit/HTTP contracts run normally; PostgreSQL tests require isolated *_test schemas. Live tests require explicit `S3_LIVE_TESTS=1` and protected S3 environment. They only create unique `integration-tests/{UUID}/` objects and clean exact created keys, with HEAD confirming deletion. They do not mutate IAM/ACL, enumerate unrelated objects or contact Gemma.

Existing document metadata editing and reindexing are preserved. An in-place binary replacement/version-upload API was not added; new binary content is uploaded as a new source, as before. Do not interpret storage migration as replacing document contents. Automated production cross-bucket restore, old-local-copy purging, CDN/presigned downloads and online multi-provider replication are outside this release.

Real Yandex tests passed: bucket HEAD; TXT/valid PDF/PNG PUT/HEAD/GET, exact sizes/types/SHA, missing key, denied anonymous GET, exact-key deletion. Live knowledge test passed PDF extraction/BGE-M3/hybrid search, foreign access denial, abandoned lease recovery with an empty local directory, reindex and delete. Live migration passed dry-run, copy/check/retry and local retention; synthetic missing originals were skipped. HTTP/PG tests separately cover denied writes/reads, corruption, cancellation, database failure after PUT and concurrent byte-quota admission. Those failure contracts are NOT assertions that IAM was changed or invalid production credentials were exercised.

Browser: `LIVE_STORAGE_APP=1 npx playwright test object-storage.spec.ts` against a dedicated *_test API configured with a unique S3_KEY_PREFIX. Checks PDF upload from cabinet, real BGE retrieval, authorized SHA download, other-tenant refusal, logo upload and visible logo in installed iframe. Artifacts remain ignored. No actual company files were migrated; no production, FinKoper or GPU access occurred.

Offline restore-local does not require a working S3 connection or S3 credentials once exported bytes are available; a PostgreSQL test covers a lost/unreachable old bucket identity. The administrative CLI validator permits its explicit deployment credential while retaining other production checks; the API validator still rejects raw administrative credentials. Local recovery helpers require STORAGE_TEST_DATABASE=confirmed_test as an operator acknowledgement.
