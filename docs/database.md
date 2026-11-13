# База данных

Этап 5 — `005_operator_inbox.sql`: control_mode/state_version/assigned_user_id и timestamps на conversations; snapshot state_version на ai_executions; nullable execution_id, role=operator, author_user_id/request_id/change_seq на прежних messages. Новые conversation_events (private notes и idempotency), conversation_reads (per-member cursor), organization_invitations (SHA-256 token, expiry/revoke/accept). Composite tenant/member/conversation FK запрещают чужое назначение/автора. Общий conversation_change_seq и BEFORE UPDATE trigger делают изменения статусов доступными cursor poll. Индексы website activity/mode/assigned, unread visitor sequence и pg_trgm message content. Старые миграции не изменялись. Мигратор проверяется повторным применением; checksum он не хранит, изменять применённый SQL запрещено правилами проекта. [Детали](operator-inbox.md).

Этап 4 — новая append-only `004_public_widget.sql`, предыдущие миграции неизменны. `widgets` хранит публичный UUID, site/assistant/org, JSON appearance, точные origins, ownership challenge, verified_at и приватный logo_key. `widget_sessions` — scoped UUID, SHA-256 token_hash, parent origin, expires_at, message_count. Составные FK гарантируют соответствие tenant/assistant/site/widget. Индексы покрывают expiry и site/time quotas.

Существующие `conversations` поддерживают `preview` и `website`: created_by nullable только для website, добавлены site_id/anonymous_session_id. CHECK гарантирует правильный канал; visitor session имеет один conversation. `conversation_messages` и `ai_executions` используются повторно без параллельных таблиц истории. Источники/версии по-прежнему в execution snapshot. Удаление site намеренно каскадно удаляет widget → anonymous sessions → website conversations → messages/executions; интерфейс требует подтверждение. Удаление организации отсутствует. Истёкшие visitor sessions блокируют доступ, но автоматический retention/GC ещё не реализован. Повторный запуск мигратора проверяется тестами через schema_migrations.

`001_foundation.sql`: users, sessions, organizations, organization_members, assistants, sites. Дополнительная техническая schema_migrations создаётся мигратором. UUID v4 генерируется криптографически в Go; даты timestamptz в БД. Email нормализуется в нижний регистр, уникален в пределах региональной БД. Пользователь может иметь множество членств.

Membership PK `(organization_id,user_id)`, индекс по user_id. Роль CHECK owner/admin/operator. Создание компании и owner выполняется одной транзакцией. API удаления компании нет. FK пользователей/компаний/ассистентов запрещают случайное удаление зависимых данных. В foundation CASCADE — сессии при явном удалении пользователя; членства блокируют такое удаление до отдельной процедуры.

Ассистенты: уникальная пара `(organization_id,id)`, ограничение status='draft'. Сайты ссылаются на эту пару составным FK: даже ошибочный SQL не сможет связать компании через ассистента. Домен уникален внутри компании, хранится в ASCII IDNA без завершающей точки. В базовой sites только status='unverified'; актуальные widget ID/verification — в widgets этапа 4. Индексы PK/UNIQUE покрывают фильтрацию ассистентов/сайтов по компании, отдельный индекс sites_assistant_idx — связи ассистента. Sessions индексированы по user_id и expires_at.

Команда `go run ./cmd/api -migrate` применяет SQL по имени в одной транзакции под advisory lock и записывает версии. Повторный запуск безопасен. Это forward-only миграции: автоматического down нет; восстановление production через проверенный backup/новую миграцию. После публикации не менять применённый SQL. Интеграционный тест дважды применяет миграцию в чистой случайной схеме отдельной БД с суффиксом `_test`.

Tenant isolation реализована в приложении и FK; PostgreSQL RLS пока не используется. Новые repository методы обязаны включать organization_id. Регионы используют независимые БД без общей таблицы пользователей или синхронизации.

## Этап 2

`002_knowledge.sql` добавляет public.vector и три таблицы: knowledge_sources (источник, текущее содержимое, checksum, revision/active_revision, enabled, язык, metadata файла/web, статус), knowledge_jobs (очередь с attempts, available_at, lease_until и fencing lease_token), knowledge_chunks (упорядоченные fragments/page, vector, model/digest/dimensions и generated simple tsvector).

Composite FK source → `(organization_id,assistant_id)` assistants; jobs/chunks → `(organization_id,assistant_id,id)` source. CASCADE применяется только к принадлежащим удаляемому источнику jobs/chunks. Миграция не изменяет и не очищает таблицы этапа 1. Файлы находятся вне БД, UUID ключ не выдаётся API.

UNIQUE upload checksum и URL ограничены компанией и ассистентом; UNIQUE(source_id,revision,position) предотвращает дубли chunks; UNIQUE(source_id,revision) — дубли jobs. Scope/queue/GIN индексы; HNSW нет. Vector без фиксированной typmod-размерности, потому что фактическая размерность проверяется у провайдера; CHECK vector_dims=dimensions. На настоящей BGE-M3 получено 1024. Поиск обязательно фильтруется model/digest/dimensions. Полная новая версия публикуется транзакционно, предыдущие chunks удаляются после смены active_revision в той же транзакции. История полных старых документов пока не хранится; старые активные chunks сохраняются до успешной замены.

Образ PostgreSQL сохранён на Alpine/major 17, расширение собирается из pgvector v0.8.7. Именованный volume остаётся прежним. Миграции forward-only, extension создаёт привилегированный мигратор. Подробнее — docs/rag.md.

## Этап 3

`003_ai_conversations.sql` добавляет assistant_ai_settings, conversations, ai_executions, conversation_messages. Старые таблицы/миграции не изменены. Settings имеют composite assistant FK; conversation — organization/assistant/id и created_by; executions/messages — composite tenant/conversation FKs. CASCADE ограничен удалением preview conversation и её executions/messages; удаления компании API по-прежнему нет.

Execution хранит request UUID/hash, provider/model, статус processing/completed/failed/canceled, wall/provider duration и nullable tokens, error_code, selected sources JSON snapshot, settings snapshot, response_language, policy_fallback и expires_at. UNIQUE(conversation_id,request_id) + partial UNIQUE processing не допускают повторного или параллельного запуска. Context/history статусы проверяются сервером. Model thinking, URL-токены и raw inference errors не хранятся.

Messages имеют monotonic bigint sequence для порядка и cursor pagination, один user и один assistant на execution. Незавершённый assistant content пуст; только успешный Finish сохраняет полный текст транзакционно. В этапе 5 prompt исключает failed/canceled AI ответы, включает завершённые visitor/staff сообщения; внутренние заметки исключены. Sources snapshot не FK на mutable knowledge_chunks: source/chunk IDs, revisions и текст остаются для истории после reindex/delete знания. При удалении preview удаляются и snapshots; перед production нужна retention-политика.

## Migration 006_leads.sql

Append-only settings/fields, frozen per-conversation lead_collections, leads and lead_events. Composite tenant foreign keys, unique collection_id and version CAS prevent cross-tenant references and duplicate confirmations. Main contacts are typed, additional validated values use JSONB. Lead deletion cascades only its events; collection tombstone prevents resubmission. Conversations remain; sites with leads cannot be deleted until those leads are reviewed/deleted. Existing migrations 001?005 are unchanged. See leads.md for privacy/retention.
