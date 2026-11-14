# Stage 9A readiness

Stage 9A.1 storage update: private S3 integration and live Yandex verification passed, including real PDF/BGE-M3/RAG, TXT/MD/DOCX processing, original download, widget logo and test backup/restore. Startup/readyz use bounded HeadBucket checks cached5seconds and reject recorded bucket identity mismatches; no health-check PUTs. Registry, missing originals, migration and orphan lifecycle are described in object-storage.md. This does not remove the existing deployment/endurance/legal-policy gates below or establish Gemma/GPU readiness.

This is a preparation report, not production certification. No deployment, FinKoper, Gemma or GPU tests were performed. Stage 9B must measure the actual model separately.

| Requirement | State / evidence | Remaining risk | Before pilot | Before public launch |
|---|---|---|---|---|
| Production configuration | Startup rejects mock, development receiver, local/test public origins, weak DB credentials and non-verify-full TLS; config unit tests | Reverse proxy/TLS and restricted DB roles must be provisioned and independently checked | yes | yes |
| Webhook encryption | Startup decrypts every stored endpoint; continuity/missing/wrong-key PG integration test | No online multi-key rotation; backup key separately | yes | yes |
| Tenant isolation, CSRF, SSRF, HMAC, widget isolation | Existing full PG/security/browser regression retained | Not a penetration test; edge abuse policy required | yes | yes |
| Storage readiness | Startup and readyz test write access; originals CLI reports missing/nonregular objects without deletion | No automatic repair; historical missing originals remain missing | review | yes |
| Retention | 500 expired user sessions per batch, 7 days after expiry; CLI dry-run; PG test | Other retention disabled; visitor-session deletion cascades conversations | approve policy | yes |
| Backup/restore | Offline standard pg_dump/pg_restore + manifest/files; see backup-restore.md | Requires stopped writers, separate secret escrow and restore drill | yes | yes |
| Metrics/logs | Token-protected metrics; aggregate labels only; server-generated request ID, status/duration, no path/body/header logging | No full worker tracing; counters reset with process; technical gauges follow retained history | review | yes |
| AI handoff fencing | Unchanged conversation lock through bounded SSE flush; existing PG contention tests | Up to 10s per blocked network flush; proxy timeouts matter | yes | yes |
| Limits | Commercial quotas shared PG, safety limiters process-local | Deploy only one API process behind edge IP limits; no multi-process safety claim | yes | yes |
| Load | Measured mixed10 clients and 10/25/50/100 configuration/list ramp; sparse CPU/RSS/PG sample | Endurance, slow-client soak and full transaction percentiles still outstanding | review | yes |
| Erasure | Existing lead/document/preview deletion; restricted conversation/member dependencies documented | Complete website visitor erasure, staff pseudonymization and company erasure not implemented | restrict pilot data | yes |
| Real AI | Mock/HTTP contracts only; real BGE-M3 regression | Gemma answer quality and GPU throughput unverified | AI disabled or explicit demo | stage 9B |

Initial critical findings: saved webhook key compatibility was not checked at startup; storage readiness was not checked; backup/restore lacked scripts. These now have implementation paths and tests. Offline restore drill passed on a NEW test database, including application-level login/company/conversation/lead/knowledge checks. Desirable findings: request correlation, protected metrics, bounded technical cleanup and originals diagnostics. Pilot-acceptable constraints: one API instance, no real model claims, no automatic erasure of customer data.

Do not advertise public production readiness until endurance/slow-client infrastructure checks and approved deletion/retention policies are resolved. Mixed 10-client load now includes sessions/SSE/replay, operator history/take/reply, external create/replay/feed with local HMAC receiver and real knowledge search; see stage9-results.json. Functional cross-tenant and fencing guarantees are additionally covered by PostgreSQL regression tests. Authentication uses server-generated opaque sessions, so there is no invented JWT signing secret. Never add production fallback secrets. Production raw billing admin tokens are rejected in the API environment.

Originals investigation: the old site_assistant_test directory scan returned15 document records,13 present and2 missing/unreadable. Exact identifiers are available through the protected operations CLI, not metric labels. A historical root mismatch or removed test fixture is possible, but cause is not proven. No invented files or metadata repair was attempted. Affected originals cannot be re-extracted; an old ready index can remain searchable, and new uploads fail closed when storage accounting is incomplete. The restored Stage9 fixture original was present and unmodified. Reconcile/restore actual originals, or explicitly delete the source with company authorization; never silently mark its size zero.
