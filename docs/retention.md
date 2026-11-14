# Pilot retention and erasure

There is no universal legal retention period. The deploying organization must approve region-specific purposes, legal bases, access and durations. These technical defaults do not establish GDPR compliance.

| Data | Current policy |
|---|---|
| Login sessions | Automatic bounded cleanup, default 7 days after expiry; RETENTION_SESSION_DAYS 1–3650 |
| Visitor sessions | Expire for authentication, retain rows; deleting them cascades conversations |
| Conversations/messages, leads, collections | Automatic deletion disabled; retain until approved manual deletion policy |
| AI executions/source references | Retain with conversation; no thinking or partial successful content |
| API keys/invitations | Expiry/revocation enforce access, records retained for audit |
| Webhook attempts/deliveries/events | Retain; recovery cursor and retry semantics must not be silently broken |
| Knowledge originals/chunks/jobs | Explicit authorized source deletion; file quota released only after physical removal |
| Billing periods/reservations/audit | Retain accounting without conversation text; completed usage is not refunded by deletion |

`cmd/operations --action cleanup --days 7` is dry-run by default. `--apply` deletes at most 500 expired login sessions per invocation using SKIP LOCKED and a 3-second statement timeout. API runs one batch hourly. Counts only are logged; no session digest/user identity. This global technical sweep requires deployment admin credentials for CLI; it cannot select a tenant or remove customer content. Active sessions are excluded. Use the billing deployment credential gate described in billing.md.

Deletion review: leads and document deletion have authorized endpoints; sources remove chunks/jobs by FK and schedule original deletion. Preview conversations can be deleted. Website conversations can reference leads, collections, session and notes: do not issue ad hoc cascade DELETE. Staff membership is referenced by assignments/authorship/audit; removing a member requires a separate approved anonymization/reassignment workflow. Those complete erasure workflows are not implemented here.

Company erasure procedure is intentionally dry-run only: stop new traffic, identify exact organization UUID/name with two-person review, inventory FKs, active executions/jobs, billing/file reservations, source originals, leads/collections/events and backup retention; export counts only; approve legal/audit exceptions before building a scoped transactional erasure operation. No universal company-delete command exists. Backups must expire under the approved policy too; removing live data does not remove old snapshots.
