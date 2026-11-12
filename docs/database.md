# База данных

`001_foundation.sql`: users, sessions, organizations, organization_members, assistants, sites. Дополнительная техническая schema_migrations создаётся мигратором. UUID v4 генерируется криптографически в Go; даты timestamptz в БД. Email нормализуется в нижний регистр, уникален в пределах региональной БД. Пользователь может иметь множество членств.

Membership PK `(organization_id,user_id)`, индекс по user_id. Роль CHECK owner/admin/operator. Создание компании и owner выполняется одной транзакцией. API удаления компании нет. FK пользователей/компаний/ассистентов запрещают случайное удаление зависимых данных. Единственный CASCADE — сессии при явном удалении пользователя; членства блокируют такое удаление до отдельной процедуры.

Ассистенты: уникальная пара `(organization_id,id)`, ограничение status='draft'. Сайты ссылаются на эту пару составным FK: даже ошибочный SQL не сможет связать компании через ассистента. Домен уникален внутри компании, хранится в ASCII IDNA без завершающей точки. Только status='unverified'; никаких ключей виджета. Индексы PK/UNIQUE покрывают фильтрацию ассистентов/сайтов по компании, отдельный индекс sites_assistant_idx — связи ассистента. Sessions индексированы по user_id и expires_at.

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

Messages имеют monotonic bigint sequence для порядка и cursor pagination, один user и один assistant на execution. Незавершённый assistant content пуст; только успешный Finish сохраняет полный текст транзакционно. История prompt исключает failed/canceled execution целиком. Sources snapshot не FK на mutable knowledge_chunks: source/chunk IDs, revisions и текст остаются для истории после reindex/delete знания. При удалении preview удаляются и snapshots; перед production нужна retention-политика.
