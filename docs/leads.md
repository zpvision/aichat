# Scenarios and leads — Stage 6

The existing Go monolith owns all decisions. `internal/leads` contains settings, validation, structured extraction, collection state and CRM services/repositories. HTTP adapters live in `httpapi/leads.go`; the existing widget and AI engine supply authentication and generation. There is no second chat engine, database, webhook sender or external CRM.

## Configuration and UI

Open a company → **Scenarios** → choose an assistant. Consultation remains the default for all existing assistants. Enable collection, configure up to 12 active/inactive fields, goal, order, four label translations and success messages. Six standard fields are supplied; Russian INN is inactive by default. Custom keys use `[a-z][a-z0-9_]{0,39}` and are independent of labels. Saved keys cannot be renamed in the editor; remove/add a field instead. `PUT lead-settings` uses revision comparison, returning `state_conflict` on stale edits.

Types: text (up to 2,000 Unicode characters), email, international phone (`+` and 8–15 digits), number (finite, globally ±1e9, optional min/max/integer), select (1–30 exact options), `inn_ru` (10/12 digits and checksum). No user-supplied regex. “Phone or email is sufficient” requires one valid contact and forbids requiring those two fields individually. Validation checks format, not ownership, reachability or registration of contacts/INN. INN checksum reference: [bank validation documentation](https://developers.tochka.com/docs/cyclops/validation).

Set a company privacy-policy URL in website chat settings before starting collection. The widget shows a small “Start a request” action. It does not require contacts for consultation. A narrow, explicit request phrase in RU/EN/CS/DE can also start collection; other intentions and agreement to an AI offer use this functional action, avoiding speculative consent detection.

## State and extraction

`idle → collecting → confirmation → submitted`, or `collecting/confirmation → canceled`. A canceled collection may restart with empty values and current settings. A conversation supports one submitted lead in this release; a subsequent request needs a new conversation. Starting freezes settings revision, fields, labels, success copy and privacy URL. Later settings changes (including disabling new collection) do not alter that snapshot.

Every value has server validation state, error code, timestamp, source and, for extraction, the actual visitor message ID/sequence. Invalid values remain visible for correction. A later explicit correction replaces an earlier value; reprocessing old messages cannot overwrite a newer manual correction. There is no model-confidence trust score.

`StructuredExtractor` is separate from free-text generation. `MockStructuredExtractor` accepts explicit JSON objects matching configured keys and four exact natural-language fixtures in `extractor.go`. Example after starting collection:

```json
{"full_name":"Anna","email":"anna@example.com","employee_count":12}
```

Add `employee_count` as a custom numeric field first. Other natural language is not analyzed intelligently by the mock. Tests also inject partial/invalid responses, failures, delay and cancellation. The UI continues to mark mock responses as demonstrations.

`OllamaStructuredExtractor` reuses the Ollama `/api/chat` adapter with `format` JSON Schema, nonstreaming, bounded output, no tools and no thinking persistence. It uses regional `AI_BASE_URL`/`AI_MODEL`, separately from BGE-M3 configuration. [Official structured-output contract](https://ollama.com/blog/structured-outputs). No GPU connection is attempted at startup. Mock remains forbidden in production. Real Gemma extraction quality has **not** been tested.

The model must return configured keys, typed candidate values, allowed visitor message IDs and verbatim evidence. Go checks the evidence occurs in the source and normalizes to the proposed value. Unknown keys, invented evidence, malformed JSON, wrong sources or conflicting proposals are rejected. This deliberately favors omission over guessing; paraphrased descriptions must be edited manually if they fail evidence checks.

Extraction runs only during active collection and AI control, at most 40 calls per collection, 2 concurrent extractions per process, 8 seconds each. Input is the last 12 completed visitor messages, at most 24 KB. There are no operator messages or notes in extraction input. Extraction precedes free-text streaming and adds latency. Existing website/session/company generation budgets and the AI concurrency semaphore still apply. Candidate values are published only in the successful AI finish transaction; failures, cancellation, handoff or a newer collection edit prevent stale publication.

## Confirmation and transactions

The widget displays a summary, edit, cancel, privacy link and explicit **Send request** action. It sends the version actually reviewed. Backend locks conversation then collection, checks AI/human mode, conversation version, collection version, configured keys, all required fields and contact rules again. Staff must take the conversation before manual completion. Operator actions require ownership; owner/admin may work on human-controlled conversations.

Lead insertion, created-history event and submitted state commit together. Unique `collection_id` guarantees one lead for concurrent/repeated confirmation. The original confirmed version can be replayed after a lost response. A DB failure rolls everything back and retains the editable collection. No model response can submit a lead. PostgreSQL handoff fencing is reused: stale extraction cannot publish after takeover, even if a provider ignores cancellation. Existing SSE delivery fencing remains unchanged.

## CRM and rights

**Leads** has status/site/assistant/assignee/date filters, keyset pagination (30 default, 50 maximum), contact details, extra fields, source, conversation link, assignment, status, notes and latest 100 audit events. Statuses: `new`, `in_progress`, `won`, `lost`. Every simultaneous patch is version checked. Every changed status, assignment, contact edit and note gets its own audit event.

Owner/admin see and manage all company leads, contacts, assignments and deletion. Operators see their own leads and unassigned open leads; they can claim an unassigned lead, then update its status/notes. They cannot change contacts, scenario settings, delete or assign to another employee. Lead assignment does not grant access to an independently assigned conversation. Public APIs expose neither CRM IDs, notes nor other-company information.

Typed name/email/phone columns store main contacts; validated JSONB holds extra fields. Provenance metadata does not duplicate contact values. Submitted collection values are cleared. Lead deletion removes the lead/history but leaves a submitted tombstone, preventing resubmission; conversation messages remain. Site deletion is blocked while leads reference it (`site_in_use`).

## Local verification and limits

Follow README, apply migration 006 with `./scripts/backend.ps1 -Migrate`, build widget and start the existing Go/backend, dashboard and independent demo page. Use `AI_PROVIDER=mock`; no extra services/models are needed. Configure fields, privacy URL and allowed origin, then use the existing installation loader on `http://localhost:4173/widget/demo/index.html?widget=PUBLIC_ID`.

Run `go test ./...` with `TEST_DATABASE_URL` ending `_test` for PostgreSQL tests; `npm test`/`npm run build` in both clients; run `npx playwright test leads.spec.ts` against a fresh local test API process. Browser artifacts are ignored under `artifacts/stage6`. The suite uses real PostgreSQL and Go with mock generation/extraction, not Gemma.

Before production: define per-region/company retention for drafts, contacts, source messages, notes, backups and erasure requests; add scheduled retention/GC and deployment-wide limits. Deleting a lead is not deletion of the original conversation. There is no marketing consent, email/SMS sending or contact verification. In-memory fast limits are single-process; PostgreSQL generation budgets remain shared. Data goes only to the configured regional AI endpoint. Prompt text and extracted contacts are not copied into execution logs.

Stage 7 now attaches transactional lead.created events and delivery jobs using the shared leads INSERT trigger. HTTP delivery remains outside the creation transaction. See finkoper-integration.md for the universal company API and signed metadata webhooks. There is no FinKoper destination, SSO or external event bus.
