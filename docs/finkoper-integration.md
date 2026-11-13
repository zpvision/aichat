# FinKoper and other CRM consumers

AI Chat is the data provider. FinKoper has no lead-creation API in this project, and AI Chat contains no FinKoper destination, field mapper, SSO client or calls to FinKoper. Any CRM can implement the consumer described here independently.

## Contract

1. An AI Chat owner/admin creates a company API key in **Integrations**. Grant `leads:read` to fetch leads; `leads:write` is separately available for explicitly creating leads. Keys expire in 1–365 days, are shown once, stored hashed, and can be revoked. Keep the raw key in the CRM's server-side secret store, never browser storage or URLs.
2. The CRM exposes its own public HTTPS webhook receiver. An administrator adds its URL in AI Chat and transfers the one-time signing secret into the CRM's secret store. This is a different secret from the API key.
3. A committed lead generates `lead.created` and a delivery job in the same PostgreSQL transaction. AI Chat sends exactly this metadata, without contact fields or conversation contents:

```json
{
  "event_id": "stable-event-uuid",
  "event_type": "lead.created",
  "lead_id": "stable-lead-uuid",
  "created_at": "2026-11-13T12:00:00Z"
}
```

`webhook.test` uses the same envelope with `lead_id: null`; it does not create a lead. No user-configurable PII payload mode is currently offered.

## Verify before accepting

Headers: `X-SiteAssistant-Timestamp` (Unix seconds), `X-SiteAssistant-Signature` (`v1=` followed by lowercase hex), `X-SiteAssistant-Event-ID`. Compute:

```text
HMAC-SHA256(signing_secret, timestamp + "." + exact_raw_HTTP_body)
```

Compare in constant time. Validate the numeric timestamp within ±300 seconds of the CRM clock; synchronize clocks. Verify the raw bytes before parsing JSON, impose body/header/time limits, and check that header/body event IDs agree. Do not reserialize the JSON before checking its signature. `internal/integrations.Verify` is a Go reference; `scripts/webhook-receiver.mjs` is a local demonstration, not a production CRM service.

Use the signing secret's literal text bytes as the HMAC key; do not base64-decode this endpoint secret. The separate server-side `WEBHOOK_ENCRYPTION_KEY` configuration is base64-decoded to an AES-256 key and must never be shared with the CRM.

The CRM should transactionally persist an inbox record with a unique `(AI Chat region, company integration, event_id)` key and then respond 2xx. Repeated deliveries must also receive 2xx. Do not acknowledge only an in-memory receipt. Process the durable record asynchronously. Delivery is **at least once**; timeout or a lost response can cause retries. Different events may arrive out of order.

## Fetch and upsert a lead

For `lead.created`, the CRM backend calls the appropriate region's AI Chat API:

```http
GET /api/v1/external/leads/{lead_id}
Authorization: Bearer sa_...
```

The key determines the organization; callers cannot select another organization. The response contains the normalized contacts, validated additional `data`, stable assistant/site/conversation IDs, status/version and timestamps. It contains no internal notes, AI instructions, token hashes or private execution data. A lead created through the API can have null site/conversation IDs. A deleted lead returns 404.

FinKoper can then map these *documented AI Chat fields* to its own internal domain model and create/update a record in its own database. That mapping must be implemented inside FinKoper after its maintainers define the target model; no unknown FinKoper fields/endpoints are assumed here. Use a unique `(region, AI Chat organization, lead_id)` external-reference key in the CRM so webhook retries and recovery imports update the same record. Do not duplicate contacts when an event is delivered again.

## Recover missed notifications

```http
GET /api/v1/external/leads?cursor=0&limit=50
Authorization: Bearer sa_...
```

Response: `items`, `next_cursor`, `has_more`. Every item contains `event_id`, `lead_id`, `created_at`, `deleted` and `lead` (current DTO or null for a deleted lead). Start without a cursor for initial synchronization; retain `next_cursor` only after all items in that page are durably applied. Continue while `has_more`; poll later using the last cursor even when the page is empty. Cursor numbers are opaque to the consumer: do not manufacture them, use timestamps as replacements or compare cursors across regions/companies. Webhook and recovery feed share stable event/lead IDs. Existing Stage 6 leads are backfilled into this feed without retroactive webhook deliveries.

Allocation is serialized per organization, so a cursor cannot skip a still-uncommitted earlier lead. The feed tracks lead *creation*, not a complete change-data-capture stream. It returns current lead data; later edits do not emit `lead.updated` yet. CRM reconciliation of subsequent edits/deletions requires an explicit future synchronization contract; deletion tombstones are visible when traversing the creation feed.

## Optional creation through AI Chat API

```http
POST /api/v1/external/leads
Authorization: Bearer sa_...
Idempotency-Key: crm_request_0001
Content-Type: application/json

{"assistant_id":"uuid","values":{"full_name":"Anna","email":"anna@example.com"}}
```

`site_id` is optional and must belong to that assistant/company. Values use the assistant's active lead-field keys and validation rules. No fake conversation is created. The request is an explicit company action; the integrating CRM is responsible for its own lawful collection/confirmation flow. The lead appears in the existing mini-CRM with source `api`, and emits the same `lead.created` event. Avoid loops when importing an event from AI Chat: fetching an existing lead should upsert the CRM's database, not POST another lead to AI Chat.

`Idempotency-Key` must contain 8–100 ASCII letters/digits/underscore/hyphen. Reuse it for identical retries, including after a network failure. Same key plus different request data returns 409. The namespace is per company, so it survives API-key rotation. Deleted confirmed requests cannot be recreated with that key. UUIDs and idempotency records must be retained by the CRM independently of delivery attempts.

## Operational behavior

- API limits: PostgreSQL-enforced 120 requests/minute/key and 600/minute/company. A process-local IP gate also limits external API requests to 600/minute; configure an edge limiter before production. 429 includes `Retry-After` for authenticated budget limits. No wildcard CORS is provided: keys are for server-to-server access.
- Webhooks: no redirects, pinned public DNS/IP on every connection, no HTTP proxy, 8-second HTTP timeout, bounded response read. All standard private/loopback/link-local/metadata targets are denied. Only 2xx counts as success.
- PostgreSQL jobs use `SKIP LOCKED`, lease fencing and crash recovery. Eight automatic attempts, delays 1s/5s/30s/120s/600s/1800s/3600s. Failed/canceled jobs can be explicitly retried from the cabinet with the same event ID. Journal stores status/code/duration, never response bodies or contacts.
- Disabled endpoints cancel queued deliveries when claimed. Re-enabling requires retrying canceled jobs or using recovery. New endpoints receive future events only. Rotating signing secrets takes effect for subsequent attempts; coordinate receiver rotation and allow the previous secret during a short controlled transition if necessary.
- RU/EU installations have independent database, keys, encryption keys, events and URLs. FinKoper must select the correct regional installation and must not copy personal data across regions implicitly.
- Before production, define retention/erasure for keys, event identifiers, attempts, tombstones, leads and backups; provision TLS, secret backups, monitoring and shared edge limits. API key hashes cannot recover lost keys; webhook ciphertext requires the stable encryption key. There is no production deployment in this stage.

## Local receiver

Generate a 32-byte encryption key and save its base64 encoding as `WEBHOOK_ENCRYPTION_KEY` in ignored `backend/.env`. Set `WEBHOOK_DEV_RECEIVER_URL=http://127.0.0.1:18086/webhook` for local development only. Restart backend, create a webhook with exactly that URL and copy its signing secret. Then run:

To generate the encryption-key value locally: `node -e "console.log(require('node:crypto').randomBytes(32).toString('base64'))"`. Keep this value stable and back it up securely; replacing it without re-encrypting endpoint secrets makes existing ciphertext unreadable.

```powershell
$env:WEBHOOK_SIGNING_SECRET = 'one-time-secret-from-cabinet'
node scripts/webhook-receiver.mjs
```

Click **Send test** and inspect the delivery journal. The exact loopback exception is forbidden in production and permits no other local path/port/IP. The demonstration receiver uses an in-memory deduplication set; a production CRM must use the durable inbox/upsert transaction described above. No FinKoper code or infrastructure is required for these tests.
