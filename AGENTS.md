# site-assistant

Stage 1 is a modular Go monolith and React/Vite client. Existing root GoLand sample is not the application; work in `backend/` and `frontend/`. No push or production actions without explicit authorization.

- HTTP adapters: `backend/internal/httpapi`. Business rules: `internal/core/service.go`, `auth.go`. SQL only in `core/store.go` and migrations. Keep the module small; split `core` by domain when complexity warrants it.
- Every tenant operation must call `Service.Access`; every entity lookup/update/delete includes organization ID. Client IDs never prove membership. Composite sites/assistants FK is required.
- Owner controls company settings. Owner/admin write assistants and sites. Operator reads only. No public membership mutation endpoints yet.
- Passwords Argon2id; server sessions stored as SHA-256 digests. Never put secrets/PII into logs or auth tokens into browser storage. Mutations require exact Origin + custom CSRF header + JSON.
- RU/EU have separate infrastructure and databases. Language is independent. All UI strings in four complete dictionaries; API returns stable machine error codes.
- No AI, RAG, files, widget, leads, billing, or integrations in this stage. `active` is forbidden. No simulated functionality.
- Append SQL migrations; never rewrite migrations already deployed. Do not truncate databases. Integration tests require `TEST_DATABASE_URL` with database name ending `_test` and use isolated schemas.
- Before handoff: in backend `go test ./...`, integration tests with test DSN, `go vet ./...`, `go build ./cmd/api`; in frontend `npm ci`, `npm test`, `npm run build`, `npm audit`. Browser tests: `npx playwright test` with local servers pointed at a test database. Regenerate OpenAPI after API changes with `node scripts/generate-openapi.mjs`.
- Do not commit `.env`, IDE settings, binaries, screenshots or node_modules. Preserve unrelated changes.
