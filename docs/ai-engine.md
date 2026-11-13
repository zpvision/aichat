# AI-движок — этап 3

Общий Go-движок работает для preview-чата; будущие каналы должны вызывать тот же service. **Генерация проверена только с MockProvider и тестовым Ollama HTTP-сервером. Поиск BGE-M3 проверен по-настоящему.** Gemma не подключалась/не скачивалась, к прежнему GPU-серверу запросов не было. Качество, скорость и модельная совместимость Gemma пока неизвестны.

## Архитектура и контракт

`internal/ai`: config, models, provider, prompt, store, service. HTTP — `httpapi/ai.go`, React — `PreviewChat.tsx`, транспорт — `chat-api.ts`. Используются прежние pgx pool, auth/CSRF и knowledge.Service.Search. Нового поиска, БД, брокера и микросервиса нет.

AIProvider.Generate принимает context, сообщения, streaming flag и callback текстовых частей. Возвращает итоговый текст и nullable usage: input/output tokens, provider duration. Внутренний JSONSchema передаётся Ollama как format для будущего structured output; публичного schema/tool/lead API нет. Mock не делает структурированное извлечение. Совместимость format с Gemma ещё не проверена.

Режимы: disabled (ai_disabled), mock (детерминированное demo), ollama (POST /api/chat). Mock запрещён конфигурацией при APP_ENV=production. В mock executions пишется model=deterministic-demo-v1, а не Gemma; токены неизвестны и остаются null. Mock имеет управляемые Delay/Failure/FailAfter для unit-тестов, но не команды в сообщении посетителя. UI явно сообщает: «Демонстрационный режим — ответы тестовые».

## Конфигурация

```env
AI_PROVIDER=mock
AI_BASE_URL=http://localhost:11434
AI_MODEL=gemma4:26b
AI_TIMEOUT_SECONDS=120
AI_MAX_OUTPUT_TOKENS=500
AI_TEMPERATURE=0.3
AI_THINK=false
AI_MAX_CONCURRENT=4
AI_HISTORY_MESSAGES=20
AI_CONTEXT_CHARS=24000
AI_RAG_CHARS=12000
AI_RAG_TOP_K=5
```

AI_BASE_URL независим от OLLAMA_BASE_URL для embeddings. Это доверенная региональная серверная конфигурация, не поле tenant API. URL — HTTP(S) origin без credentials/query, redirects и env HTTP proxy отключены. RU/EU должны иметь собственные endpoints, БД и файлы; cross-region fallback отсутствует. Обычный запуск не делает probes к Gemma. Для production явно выбрать disabled или ollama.

Общий deadline retrieval + generation — 1–120 секунд. Capacity по умолчанию 4 на процесс; занятость возвращает ai_busy без неограниченной очереди. При нескольких репликах понадобится общий inference/edge limit. num_predict/temperature/think передаются буквально и требуют проверки на конкретной модели. Текущее имя gemma4:26b — конфигурация для будущего подключения, не подтверждение установленной модели.

## Инструкции, язык и бюджет

PromptBuilder формирует: platform system rules → subordinate company configuration JSON → отдельное user-role сообщение UNTRUSTED_KNOWLEDGE_DATA_JSON → завершённая история → новый user question. RAG не становится system instructions. Правила запрещают выдумывать факты, исполнять действия и раскрывать reasoning. Instructions компании не дают серверных полномочий.

Owner/admin задают display_name, role, company_description, style, instructions, no_answer, language_policy, response_locale и offer_operator. Лимиты Unicode-символов: имя 120, роль 200, описание 2000, стиль 300, инструкции 3000, no_answer 600. Вопрос — до 2000 символов; существующий поиск получает безопасный UTF-8 префикс до 2000 байт, оригинал сохраняется полностью.

Язык не зависит от UI: auto использует лёгкую эвристику RU/EN/CS/DE, а при неопределённости инструктирует модель определить язык сообщения с fallback на язык ассистента. fixed использует response_locale, default — default_locale ассистента. Visitor language — обнаруженный язык или пустая строка. execution.response_language — выбранная языковая инструкция, а не доказательство фактического языка ответа Gemma. Смешанные/короткие сообщения могут определяться неточно.

Бюджет по умолчанию 24000 Unicode-символов: приоритет system/config и нового вопроса, затем RAG до 12000, затем суффикс до 20 сообщений истории. История не режется внутри сообщения; RAG режется по runes, сохраняется именно переданный текст. Это приблизительный budget, не tokenizer Gemma; реальные context/tokens нужно проверить позднее. Суммаризации нет.

## RAG и fallback

Используется существующий hybrid search с tenant/assistant scope. Проверенный порог retrieval не менялся, нового подобранного cosine cutoff нет. AI_RAG_TOP_K ограничен 1–5 текущим retrieval. В execution.sources сохраняются chunk/document IDs, revision, model/digest, URL/PDF page, scores и текст фактического контекста.

Если sources отсутствуют или не помещаются в budget, policy_fallback=true. Mock возвращает demo/fallback. В ollama режиме сервер возвращает no-answer текст **без вызова модели**, чтобы модель не выдумывала сведения без контекста. Default — отсутствие подтверждённой информации и предложение обратиться к сотруднику, если разрешено. Если BGE-M3 недоступна, возвращается ошибка; это не интерпретируется как отсутствие знаний.

UI: «Источники, переданные AI» — подготовленный контекст, не автоматически подтверждённые цитаты и не подтверждение удалённой доставки при сетевом сбое. При частично релевантных источниках качество ответа ещё требует проверки. Prompt instructions не дают математической защиты от semantic injection; жёсткие серверные границы — отсутствие tools/actions, scoped retrieval и авторизация. Реальный red-team Gemma ещё нужен.

## Хранение и состояния

003_ai_conversations.sql: assistant_ai_settings, conversations, ai_executions, conversation_messages. Composite tenant FKs на каждом уровне. Preview-история общая для членов компании; operator читает, owner/admin создают/send/cancel/delete/configure. Для website в будущем потребуется отдельная авторизация и расширение kind CHECK, а не второй AI-движок.

Begin блокирует conversation row. UNIQUE(conversation_id,request_id) + SHA-256 content обеспечивают idempotency: тот же UUID/body возвращает replay, другая content — request_conflict. Processing replay — request_in_progress. Failed/canceled replay не вызывает модель повторно; новое явное сообщение требует новый UUID. Partial UNIQUE допускает лишь один processing execution на диалог между процессами.

User message сохраняется до retrieval. Assistant message — processing с пустым content. Потоковые части в БД не пишутся. Только полный валидный ответ после done становится completed транзакционно вместе с execution. При error/timeout/disconnect/cancel content пустой; user/execution diagnostics остаются, но failed/canceled пары исключены из следующего prompt. Thinking field игнорируется и не сохраняется. Raw ошибки сервера/модели и содержимое запросов в логи не попадают.

При classification ошибок приоритет у context, даже если embedding adapter обернул cancel своей ошибкой. Cancel отзывает локальный context и меняет DB state; другие процессы замечают cancel/delete максимум через секунду. Finish разрешён только для processing, отменённый execution не может стать completed. Гонка с уже завершённым ответом даёт идемпотентный cancel. Recovery каждые 5 секунд помечает просроченные execution (timeout + 15 секунд) failed/ai_interrupted; автоматического повторного inference нет.

GET history: последние 100 сообщений, next_before для предыдущей страницы; bounded context query отдельный. Список диалогов пока ограничен 100 последними. Delete удаляет messages/executions/source/settings snapshots. Удаление знания не удаляет архивный snapshot из переписки: до production нужна политика retention/персональных данных. Публичных посетителей, handoff и CRM нет.

Execution API также возвращает request_id: frontend связывает network recovery с конкретной отправкой, не со старой ошибкой другого execution. Загруженные более ранние страницы истории сохраняются при обновлении свежих messages; обновлённый статус по тому же ID заменяет прежний. Cancel доступен и для processing execution, загруженного из истории без активного локального stream.

## Streaming

POST messages/stream через fetch с HttpOnly cookies, JSON, точным Origin и X-CSRF-Protection: 1. Токенов в URL нет. События: start (processing AIResult), delta ({text}), complete (итоговый AIResult/replay), error ({code}). Ошибки до start — обычный HTTP JSON. Complete отправляется после сохранения. Есть также non-streaming messages endpoint.

Ollama NDJSON проверяется на JSON, role, terminal done, content size, tool_calls и статистику. Поток без done — ai_incomplete, незавершённый текст не сохраняется как successful. Лимиты: 2 MiB wire response, 1 MiB NDJSON line, 256 KiB assembled text плюс service output-character cap. Thinking не пересылается клиенту; отрицательные usage значения отклоняются. Callback/Abort прекращают upstream при уходе из чата. React декодирует incremental UTF-8, поддерживает Enter/Shift+Enter, cancel, историю и удаление.

Reverse proxy должен отключить buffering и иметь idle timeout больше AI_TIMEOUT_SECONDS. Heartbeat/resume-event replay пока нет. Полный ответ, завершённый до обрыва доставки complete-event, можно получить через history/idempotent replay.

## Проверки и будущее GPU-подключение

Из backend с готовыми PostgreSQL/Poppler/BGE-M3: TEST_DATABASE_URL существующей *_test БД, PDF_TESTS=1, PUBLIC_IMPORT_TESTS=1, REAL_EMBEDDINGS=1; `go test ./... -count=1 -v`, `go vet ./...`, `go build ./cmd/api`. Tests проверяют Mock, HTTP-контракт Ollama, prompt/языки/лимиты, историю/replay/concurrency/isolation/recovery/fallback и real BGE-M3 revision 1/2 → Mock AI. Никакого реального /api/chat Gemma в тестах нет.

Frontend: `npm test`, `npm run build`, `npx playwright test` с API на существующей *_test БД, AI_PROVIDER=mock, KNOWLEDGE_FILE_ROOT=./data/knowledge-test. Preview browser test: реальные retrieval/knowledge и только mock generation; история после reload, независимость языка от UI, отмена, 360/768/1440.

Когда GPU вернётся, отдельным разрешённым заданием: подтвердить регион/адрес и фактический model tag; настроить AI_PROVIDER=ollama, AI_BASE_URL, AI_MODEL, сохранив отдельный BGE-M3 endpoint; проверить think/options/format/context/tokenizer и NDJSON/cancel на этой модели; затем выполнить RU/EN/CS/DE, cross-language, grounding/abstention, red-team и нагрузочные испытания. До этих проверок не утверждать качество Gemma и не активировать публичный канал.

Официальный контракт: [Ollama /api/chat](https://docs.ollama.com/api/chat).

Этап 4 добавил website канал в тот же Conversation/AIService: visitor token/origin/session разрешаются через AI.WebsiteScope, затем общий generate, SQL begin/history/prompt/RAG/provider/finish. Существующий cookie preview API не принимает website conversation. Public адаптер фильтрует SSE и историю до visitor messages; внутренние sources остаются в execution. PG session/site/org budgets дополняют общий AI concurrency limit. Отключение widget/expiry отменяют активное выполнение. Подробности — [widget.md](widget.md). Реальные ответы Gemma по-прежнему не проверены.
