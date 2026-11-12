# site-assistant

Stages 1–2 use a modular Go monolith and React/Vite client. Existing root GoLand sample is not the application; work in `backend/` and `frontend/`. No push or production actions without explicit authorization.

- HTTP adapters: `backend/internal/httpapi`. Foundation business rules: `internal/core/service.go`, `auth.go`. Knowledge rules/worker: `internal/knowledge`. SQL only in repository methods (`core/store.go`, `knowledge/store.go`, `knowledge/extracted.go`) and migrations. Keep the module small.
- Every tenant operation must call `Service.Access`; every entity lookup/update/delete includes organization ID. Client IDs never prove membership. Composite sites/assistants FK is required.
- Owner controls company settings. Owner/admin write assistants and sites. Operator reads only. No public membership mutation endpoints yet.
- Passwords Argon2id; server sessions stored as SHA-256 digests. Never put secrets/PII into logs or auth tokens into browser storage. Mutations require exact Origin + custom CSRF header + JSON.
- RU/EU have separate infrastructure and databases. Language is independent. All UI strings in four complete dictionaries; API returns stable machine error codes.
- Stage 2 includes knowledge/files/import/retrieval with Ollama embeddings only. No generated answers, Gemma, widget, leads, billing, or CRM. `active` is forbidden. No simulated inference. Tests must clearly separate mocks from real embeddings.
- Knowledge: composite organization/assistant/source FKs; authorize every route with Knowledge.Access. Atomic active_revision publication only after validated embeddings for the whole revision. Compare model digest/dimensions; never mix models. Lease tokens fence stale workers, timeouts must remain shorter than the 5-minute lease.
- All importer requests use pinned public DNS IPs, no proxy, bounded redirects/body/time, and robots.txt checks. Never bypass SSRF checks for a development test URL. Test transport mocks do not weaken the production transport.
- Files use random keys under KNOWLEDGE_FILE_ROOT; never serve originals publicly. PDF uses a resource-limited, nonnetworked Poppler CLI container. Build with `docker compose build extractor`; never remove PostgreSQL volumes. See docs/rag.md.
- Append SQL migrations; never rewrite migrations already deployed. Do not truncate databases. Integration tests require `TEST_DATABASE_URL` with database name ending `_test` and use isolated schemas.
- Before handoff: in backend `go test ./...`, integration tests with test DSN, `go vet ./...`, `go build ./cmd/api`; in frontend `npm ci`, `npm test`, `npm run build`, `npm audit`. Browser tests: `npx playwright test` with local servers pointed at a test database. Regenerate OpenAPI after API changes with `node scripts/generate-openapi.mjs`.
- Do not commit `.env`, IDE settings, binaries, screenshots or node_modules. Preserve unrelated changes.
- Full RAG checks (backend): TEST_DATABASE_URL ending `_test`, PDF_TESTS=1, PUBLIC_IMPORT_TESTS=1, REAL_EMBEDDINGS=1, optional RAG_REPORT_PATH absolute path; `go test ./... -count=1 -v`. Actual Recall@5 corpus is testdata/multilingual.json; never replace real results with mocks. Browser knowledge scenario also uses real Ollama and a test DB. Repeat full browser suites with a fresh local API process or wait for the login rate limit.
