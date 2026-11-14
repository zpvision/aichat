# Offline backup and restore

Local drill 2026-11-14 passed: writers stopped, pg_dump + originals/widget files, SHA-256 verification, pg_restore into NEW site_assistant_stage9_restored_test. Real Go API verified login and company/conversation/lead/article reads. No pending knowledge jobs or non-local webhook endpoints existed before starting the restored test API. Existing dev/test databases were not overwritten. Fixture metadata stays in ignored artifacts/stage9/restore-fixture.json; scripts/verify-restored.mjs verifies those fixture records. This is not a measured RPO/RTO guarantee.

Run from repository root. Current script targets the local Compose PostgreSQL service, not arbitrary production servers. Docker/pg_dump/pg_restore versions must match the server. Do not store snapshots or secrets in Git. Backup contains personal data: restrict OS ACLs, encrypt at rest and during transfer; stage9 artifacts are ignored but are not a security boundary.

Stop API, workers and every other writer before snapshot. Explicitly set KNOWLEDGE_FILE_ROOT and WIDGET_FILE_ROOT to the deployment directories and BACKUP_WRITERS_STOPPED=yes. Use an absolute, new private destination:

```
node scripts/backup-restore.mjs backup ABSOLUTE_DESTINATION SOURCE_DATABASE
node scripts/backup-restore.mjs verify ABSOLUTE_DESTINATION
node scripts/backup-restore.mjs restore ABSOLUTE_DESTINATION NEW_DATABASE_test
```

Backup uses custom-format pg_dump plus immutable original/widget files and SHA-256 manifest. Verification detects corruption, not malicious replacement of both manifest and backup. Symlink inputs are rejected. The script never copies .env or encryption keys. Securely escrow deployment configuration, WEBHOOK_ENCRYPTION_KEY, DB credentials and TLS material separately. Loss of that key makes webhook ciphertext unrecoverable. No S3 adapter is currently implemented.

The local wrapper buffers the dump with a 512MiB cap and fails above it; it is intended for pilot-sized snapshots. Larger server installations must use native pg_dump streaming as described below. Failed backup/restore attempts leave their new destination for inspection; choose a new destination rather than overwriting or automatically dropping data.

Restore creates a NEW database ending _test and fails if it exists. Never restore over dev/production. Point restored API storage roots at restored knowledge/widget directories, preserve the same webhook encryption key and region. Keep outbound workers/network disabled at the infrastructure layer until restored webhook URLs are reviewed. Test login, organization, conversations, leads, knowledge and original diagnostics. A database restore alone is not an application acceptance test.

For a future server: use the same standard pg_dump/pg_restore commands with a restricted backup role and PGPASSFILE, never password argv; stream to private encrypted storage, keep a coordinated immutable file snapshot and escrow secrets. Proposed pilot policy subject to approval: nightly backup plus before migrations, 7 daily and 4 weekly snapshots, monthly restore drill. RPO target 24h, RTO target 4h: targets, not measured guarantees. Online/PITR backup and remote storage are not configured.

Key rotation: current ciphertext has no key ID. Stop all API/workers, back up DB and old key, test decrypt/re-encrypt every endpoint with its original tenant/endpoint AAD inside one maintenance transaction, verify against a restored DB, then change all processes together. No online rotation CLI is provided; do not simply replace the environment key. Startup deliberately fails on wrong/missing keys when encrypted rows exist. No key is generated automatically.
