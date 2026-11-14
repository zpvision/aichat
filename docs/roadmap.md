# Следующие этапы

1. База знаний и RAG — реализовано в этапе 2: tenant-scoped статьи/документы/web, PostgreSQL queue, pgvector + BGE-M3, hybrid search и реальные multilingual-тесты. Подробнее docs/rag.md.
2. AI-движок — реализован программный этап 3: provider contract, explicit mock/disabled/Ollama adapter, bounded RAG prompt, preview conversations/history/SSE/cancel. Настоящая Gemma 4 26B пока не подключена; доступность, лицензия, параметры и качество потребуют отдельной проверки после возвращения GPU.
3. Публичный чат-виджет — программный этап 4 реализован: ленивый loader/iframe, настройки/preview, проверка домена, visitor sessions, website conversations, общий AI/RAG/SSE и ограничения ресурсов. Проверки на Mock + реальном BGE-M3; настоящая Gemma по-прежнему недоступна. Подробнее docs/widget.md.
4. Диалоги и операторы — этап 5 реализован: website inbox, роли/назначения, PostgreSQL-fenced AI/human handoff, staff messages, notes/read state, cursor polling и одноразовые приглашения. Mock-generation и настоящий BGE-M3, без GPU/CRM. Подробнее docs/operator-inbox.md.
5. Scenarios and leads ? implemented in Stage 6: configurable fields, frozen state, structured-extraction contract, explicit confirmation and mini-CRM. See leads.md.
6. Universal integrations ? implemented in Stage 7: AI Chat supplies the lead API and signed metadata webhooks. FinKoper and other CRMs consume these contracts and persist records in their own databases. No CRM-specific outbound adapter or SSO.
7. Аналитика и биллинг: региональные провайдеры и учёт использования.
8. Production RU/EU и нагрузочные тесты: независимые инфраструктуры, безопасность, backups, наблюдаемость.

Stage 6 is implemented; Stage 7 requires a separate request. Production retention/erasure, shared limits, load tests and real Gemma evaluation remain outstanding.

## Stage 6 update

Configurable collection, confirmation and mini-CRM are implemented in the existing architecture; see leads.md. Stage 7 (FinKoper/open API integrations) is a separate future task: no webhooks or outbound CRM calls are made. Production retention/erasure, shared limits, load tests and real Gemma quality assessment remain outstanding.

## Stage 7 update

Universal company API and signed outgoing lead.created webhooks are implemented. FinKoper has no lead-creation API here and is a future consumer of this contract, as are other CRMs. No FinKoper client/adapter, field mapping or SSO is implemented. Next stages require separate authorization. Production retention, key management/rotation procedures, load testing and deployment-wide edge limits remain required.

## Stage 8 update

Regional Trial/Start/Business catalogs, company subscriptions, UTC usage periods, atomic AI reservations, resource/storage quotas, external API plan budgets, audited administrative CLI and the multilingual Plan and usage screen are implemented. Prices and payments remain deliberately unconfigured. Mock responses have only demo protection; real Gemma quality/cost evaluation remains pending. See billing.md.

Future work requires a separate request: real payment providers and price approval for RU/EU, production retention and erasure, database-role hardening, load testing/observability, distributed edge abuse controls and real GPU evaluation. No subsequent stage is started automatically. Earlier Stage 6/7 future-task notes above are historical.


## Stage 9A

Stage 9A operational baseline is implemented locally: startup hardening, ciphertext checks, protected metrics, bounded session cleanup, originals diagnostics, offline backup/restore and short load samples. See production-readiness.md for open gates; this does not authorize public deployment. Real Gemma/GPU evaluation belongs to separately requested Stage9B.


## Stage 9A.1

Stage9A.1 implemented private S3 originals/logos, mixed registry, scoped original download, safe migration/export and live Yandex/application/restore tests. Existing Stage9A deployment/legal/endurance gates remain. Stage9B Gemma/GPU testing requires a separate request; no new AI provider or production deployment was started.

## Stage 9A.2

Cosmic Violet marketing redesign is implemented: isolated public landing, original product/SVG compositions, four-language scripted demo, real registration/Trial CTA and responsive visual checks. See marketing-design.md and marketing-results.json. Cabinet/widget/storage business behavior is unchanged. The next requested design direction is the working Analytics section in Cosmic Violet; it is not implemented or started here. Real Gemma/GPU work and production deployment still require separate authorization.
