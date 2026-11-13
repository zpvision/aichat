# Plans, subscriptions and usage — Stage 8

`internal/billing` extends the existing Go monolith. PostgreSQL is authoritative for catalog, subscriptions, UTC periods, resource counters, generation reservations and administrative audit. There is no payment processor, fake payment, global user subscription or billing microservice.

## Catalog and regions

Migration 009 seeds Trial, Start and Business version 1 for RU and EU. Every limit is a nullable nonnegative bigint: **0 prohibits**, **NULL means no plan limit**. Platform abuse/safety limits remain separate, including process generation slots, widget/session/hour limits, upload size, knowledge job count and per-key/IP request limits. Commercial settings have `price: null`; the UI displays no invented price or nonworking payment button.

These are demonstration settings, not final commercial offers:

| Limit | Trial | Start | Business |
|---|---:|---:|---:|
| Assistants | 3 | 5 | 20 |
| Sites | 3 | 5 | 20 |
| Members, including owner | 5 | 10 | 50 |
| Knowledge sources | 30 | 200 | 500 |
| Original files | 50 MiB | 500 MiB | 5 GiB |
| Real AI responses/period | 100 | 1,000 | 10,000 |
| Demo responses/period | 500 | 1,000 | 2,000 |
| Company concurrent generations | 2 | 4 | 8 |
| Company external API requests/minute | 120 | 300 | no plan limit |

Edit catalog through controlled SQL maintenance, not Go code. Prefer inserting a new `(region, code, version)` row and using the administrative change-plan command. Each usage period freezes its assigned limits; editing a catalog row does not silently rewrite an already assigned period. Existing subscriptions keep their selected catalog version until explicitly changed. Features are catalog descriptions of included capabilities; typed limits perform enforcement, not an arbitrary rules engine.

The database records a deployment region. Backend/CLI reject mismatched APP_REGION. A composite subscription/plan-region foreign key also prevents assigning an EU catalog plan to an RU subscription. RU/EU databases and infrastructure remain independent.

## Trial and periods

New organizations receive Trial transactionally with organization creation. `BILLING_TRIAL_DAYS` defaults to 14 (1–90), affects new organizations only, and is validated on startup. Existing organizations are backfilled with Trial starting at migration time, preserving all data and actual entity counts. Existing resources over the demonstration limits are retained; new creation is denied until usage fits or an administrator grants another plan. Old executions are not retroactively charged.

Statuses: trialing, active, expired, suspended. Actual valid_until is checked even if a stored status has not yet been refreshed. Expiration is persisted lazily by a successful billing read/check. No data is deleted at expiration. Login, profile, company access, CRM, manual operator replies, handoff and closing conversations remain available. Existing entities may be edited/deleted; entity creation still respects assigned caps. External API access retains its assigned request cap. New provider operations, including demo generation, require an active/trialing subscription.

All timestamps and boundaries are UTC. Paid periods are fixed 30-day periods by default (`period_days` is configurable), not browser-local calendar months. Period rollover is lazy and transactional. Trial has one period ending with Trial. History is retained; the cabinet shows the latest 24 periods.

Immediate CLI plan changes within an open period replace its plan/limit snapshot while **preserving used and reserved counts and the period start**. The end is retained, or shortened to the new validity end. Extending Trial extends that same period without resetting usage. Extending a paid subscription changes validity, with normal rollover at the period boundary. Reactivation after a closed period creates a new period; old usage/reservations remain attached to the original period. No monetary prorating occurs. Existing admitted generations may finish after a change/expiry and settle against their original reservation; lowering a cap does not erase usage or cancel valid in-flight work.

## AI reservations and technical data

One successfully completed genuine provider response is one commercial unit. Mock uses a separate demo bucket; failed/canceled requests, manual replies, visitor messages, replayed request IDs, repeated SSE delivery and server-only fallback do not consume commercial units. Successful demo responses count only toward demo protection. Existing public session/IP/hour limits also protect failed/canceled traffic.

Before invoking the provider, PostgreSQL locks conversation then execution then subscription, checks subscription validity, period balance and company-wide concurrency, and reserves one unit keyed by execution UUID. No transaction remains open through inference. Structured extraction in an active lead flow reserves the same execution before its model call, then generation reuses that reservation; it never creates a second commercial unit.

Successful execution completion atomically settles the reservation and marks `ai_executions.commercial_counted`. Errors/cancel/handoff release it. Existing PostgreSQL fencing still prevents a stale generation from completing after human takeover. Execution expiry/recovery and execution deletion release abandoned active reservations. Deleting a completed preview does not undo previously used quota. Billing reservation records intentionally retain opaque execution IDs independently of chat retention.

Existing `ai_executions` retain model/provider, assistant, channel through conversation, duration, exact nullable token reports, status and new `provider_invoked`/`commercial_counted` flags. There is no duplicate execution table or conversation content in new billing tables. `provider_invoked` denotes an authorized provider attempt; it does not prove the model accepted a failed network call. Technical summaries use retained execution records; deleting a conversation can remove its diagnostic execution/token detail while period business totals remain. Token sums include only exact reported values and can be incomplete; no estimates are presented as facts. Separate extractor token-cost reporting is not added in this release.

For public quota/expiry errors, the visitor message is already persisted. The HTTP adapter masks financial error codes as `ai_unavailable`; history remains available and staff can take over. Cabinet errors use `plan_limit_exceeded` or `subscription_inactive` with translated explanations and the Plan and usage screen.

## Resource limits and file accounting

AFTER INSERT/DELETE triggers on assistants, sites, organization_members and knowledge_sources update counters under the subscription lock. Only committed inserts count: duplicate invitation acceptance cannot consume another seat. Concurrent creation of the last resource is serialized in PostgreSQL across Go instances. Pending invitations are not members; acceptance can fail at capacity without consuming the invitation. Draft/disabled resources count until deletion. Existing over-cap resources can be edited; edits do not create additional slots.

The byte metric is **the actual length of original uploaded document files assigned to the company**, including originals awaiting physical deletion. It excludes article/web text, PostgreSQL chunks, embeddings and filesystem overhead; source/text/upload-size safety caps still apply. Originals are immutable; reindexing adds no file bytes. Retained old index revisions do not create original-file copies.

Upload bytes are atomically reserved before filesystem writes. The reserved UUID is also the known storage key, so crash cleanup can find an interrupted upload. Source insertion attaches that reservation in the same transaction; a failed/duplicate insertion releases it only after deleting the tentative file. An uncertain post-commit response cannot delete an already attached original. Deleting a source schedules physical deletion; failed deletion remains charged and retries in the existing knowledge worker. Abandoned uploads are fenced as deleting after a ten-minute lease; cleanup has a one-minute retry lease. No new storage service is introduced.

At startup, legacy originals are measured in bounded metadata batches and registered without deleting anything or enforcing new caps retroactively. Missing/unreadable old files are never treated as zero-size files: their company shows incomplete accounting and new uploads fail closed until the files are restored/reconciled (restart performs another scan). Cabinet/manual chat remain usable. Old unreferenced filesystem orphans without tenant metadata are outside this metric and are not automatically deleted.

## External API limits

The Stage 7 IP gate, per-key budget and company abuse budget are retained. The assigned plan adds a PostgreSQL company cap in that same authentication transaction. No plan limit is not an exemption from the platform's 600/minute company/IP or 120/minute/key safety budgets. No Redis is required. Cabinet cookies and manual operator messaging do not pass through external-key budgets.

## Administrative CLI

There is no HTTP grant/change/extend endpoint. CLI operators need deployment/DB access plus an environment credential check: runtime `BILLING_ADMIN_TOKEN_HASH` is SHA-256 hex of a high-entropy secret; supply its raw `BILLING_ADMIN_TOKEN` only to the CLI environment, never to the API process, argv, browser or logs. This gate complements trusted OS/database permissions and is not a replacement for production DB-role separation.

From backend, with the existing regional environment loaded:

```powershell
go run ./cmd/subscription --action inspect --organization UUID --expect-name "Exact company name"
go run ./cmd/subscription --action grant --organization UUID --expect-name "Exact company name" --plan start --days 30 --request-id OPERATION_UUID
go run ./cmd/subscription --action change-plan --organization UUID --expect-name "Exact company name" --plan business --days 90 --request-id OPERATION_UUID
go run ./cmd/subscription --action extend --organization UUID --expect-name "Exact company name" --days 30 --request-id OPERATION_UUID
go run ./cmd/subscription --action suspend --organization UUID --expect-name "Exact company name" --request-id OPERATION_UUID
```

Reusing request ID + identical arguments is idempotent; different arguments conflict. Organization UUID and exact expected name are mandatory. Audit stores OS actor, action and before/after subscription state, not tokens or chat. Inspect validates region/name and does not modify global trial settings. There is no real payment or assertion that a CLI grant represents a paid invoice.

## Verification and known limits

Use the existing *_test PostgreSQL database and isolated schemas. Tests cover ten concurrent requests with three slots across PostgreSQL connections, duplicate requests, demo/fallback exclusions, cancel/recovery, manual replies after expiration, concurrent resource creation, zero/unlimited values, file quotas/deletion retry, region mismatch and CLI audit/idempotency. The forced-process-restart test now crashes after the first streamed delta and asserts reservation recovery without charging. Browser billing tests use real Go/PostgreSQL and Mock only. Local Ollama HTTP contract servers test genuine-provider accounting without contacting Gemma.

No Gemma answer quality, production load, production billing/DB roles or payment systems are verified. Single-process abuse caps remain; business-critical counts/reservations are shared PostgreSQL state. Monetary prices, invoices, payment providers, automatic renewals and production deployment remain future work.

The existing local test database contains some legacy source records whose originals are absent from the test storage directory. Reconciliation reports incomplete accounting for those companies and preserves their records; new test organizations and isolated integration schemas are unaffected. Restore the missing originals before enabling new uploads for an affected company. No data was deleted to conceal this condition.
