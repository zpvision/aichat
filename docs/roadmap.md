# Следующие этапы

1. База знаний и RAG — реализовано в этапе 2: tenant-scoped статьи/документы/web, PostgreSQL queue, pgvector + BGE-M3, hybrid search и реальные multilingual-тесты. Подробнее docs/rag.md.
2. AI-движок — реализован программный этап 3: provider contract, explicit mock/disabled/Ollama adapter, bounded RAG prompt, preview conversations/history/SSE/cancel. Настоящая Gemma 4 26B пока не подключена; доступность, лицензия, параметры и качество потребуют отдельной проверки после возвращения GPU.
3. Публичный чат-виджет — программный этап 4 реализован: ленивый loader/iframe, настройки/preview, проверка домена, visitor sessions, website conversations, общий AI/RAG/SSE и ограничения ресурсов. Проверки на Mock + реальном BGE-M3; настоящая Gemma по-прежнему недоступна. Подробнее docs/widget.md.
4. Диалоги и операторы — этап 5 реализован: website inbox, роли/назначения, PostgreSQL-fenced AI/human handoff, staff messages, notes/read state, cursor polling и одноразовые приглашения. Mock-generation и настоящий BGE-M3, без GPU/CRM. Подробнее docs/operator-inbox.md.
5. Scenarios and leads ? implemented in Stage 6: configurable fields, frozen state, structured-extraction contract, explicit confirmation and mini-CRM. See leads.md.
6. FinKoper и открытый API: независимые адаптеры, аутентификация интеграций и retries.
7. Аналитика и биллинг: региональные провайдеры и учёт использования.
8. Production RU/EU и нагрузочные тесты: независимые инфраструктуры, безопасность, backups, наблюдаемость.

Stage 6 is implemented; Stage 7 requires a separate request. Production retention/erasure, shared limits, load tests and real Gemma evaluation remain outstanding.

## Stage 6 update

Configurable collection, confirmation and mini-CRM are implemented in the existing architecture; see leads.md. Stage 7 (FinKoper/open API integrations) is a separate future task: no webhooks or outbound CRM calls are made. Production retention/erasure, shared limits, load tests and real Gemma quality assessment remain outstanding.
