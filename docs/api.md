# REST API

Этап 5 добавляет company-scoped website inbox (`/organizations/{id}/conversations`, detail/messages/take/assign/resume-ai/close/notes/read), `/inbox-summary`, `/members`, invitations GET/POST/DELETE и `/invitations/accept`. Все пути под `/api/v1`, cabinet cookie/Origin/CSRF сохранены. Owner/admin работают со всей компанией; operator — со своими и свободными открытыми. Preview для operator теперь 403. Action требует request_id и текущий version, reply — также content. Публичный `/public/widgets/{widgetId}/sessions/{sessionId}/request-human` использует visitor token/origin; GET messages поддерживает after/before и mode/events. Human/disabled visitor POST возвращает SSE complete только с сохранённым user message, без AI execution. Публичные заметки/назначения/tenant IDs отсутствуют. [Полный протокол](operator-inbox.md), [OpenAPI](openapi.json).

Полный машиночитаемый контракт: [openapi.json](openapi.json), OpenAPI 3.0.3. Генерация: `node scripts/generate-openapi.mjs` из корня. Ответы ресурсов — JSON-объекты, списки — массивы (включая пустой `[]`), ошибки — `{"error":{"code":"validation"}}`. Тексты локализует клиент. PATCH частичный; неизвестные JSON-поля отклоняются. Ограничение тела 32 KiB. DELETE принимает `{}` с Content-Type JSON. Чужие tenant ID дают 404.

| Метод | Путь | Доступ |
|---|---|---|
| GET | /healthz, /readyz | Публичный |
| GET | /api/v1/config | Публичный бренд/регион/язык |
| POST | /api/v1/auth/register | Публичный, 202 без сессии; одинаково для существующей почты |
| POST | /api/v1/auth/login | Публичный, 200 + cookie |
| POST | /api/v1/auth/logout | Сессия, отзыв cookie |
| GET | /api/v1/auth/me | Сессия |
| GET, PATCH | /api/v1/profile | Свой профиль; менять имя/язык |
| GET, POST | /api/v1/organizations | Свои компании / создание компании |
| GET | /api/v1/organizations/{id} | Участник |
| PATCH | /api/v1/organizations/{id} | Owner |
| GET | /api/v1/organizations/{id}/assistants | Участник |
| POST | /api/v1/organizations/{id}/assistants | Owner/admin |
| GET | /api/v1/organizations/{id}/assistants/{assistantId} | Участник |
| PATCH | /api/v1/organizations/{id}/assistants/{assistantId} | Owner/admin |
| GET | /api/v1/organizations/{id}/sites | Участник |
| POST | /api/v1/organizations/{id}/sites | Owner/admin |
| DELETE | /api/v1/organizations/{id}/sites/{siteId} | Owner/admin |

Все мутации требуют Origin=PUBLIC_APP_URL, `X-CSRF-Protection: 1` и Content-Type=application/json. Не хранить cookie вручную в localStorage. Сервер не выдаёт access tokens. Создание компании/ассистента/сайта — 201; регистрация — 202; остальные успешные действия — 200. Readiness при недоступной БД — 503.

## Knowledge API (этап 2)

Базовый путь `K=/api/v1/organizations/{id}/assistants/{assistantId}/knowledge`. Все операции требуют сессии и членства; изменения — owner/admin. Каждый documentId дополнительно ограничен компанией и ассистентом. List/get/search доступны operator. Все ответы и ошибки — существующий JSON-контракт.

| Метод | Путь | Назначение |
|---|---|---|
| GET | K | Список metadata (content в списке пустой) |
| POST | K/articles | Создать статью `{title,content,language?,enabled?}`, 202 |
| POST | K/upload | Multipart с одним полем file TXT/MD/PDF/DOCX, 202 |
| POST | K/import-url | `{url,crawl?}`; очередь импорта, 202 |
| GET | K/{documentId} | Извлечённый текст и metadata |
| PATCH | K/{documentId} | Частичные title/language/enabled; content только у статьи, 202 |
| DELETE | K/{documentId} | Удалить источник, jobs/chunks и локальный оригинал |
| POST | K/{documentId}/reindex | `{}`, новая revision; web вручную обновляется, 202 |
| POST | K/search | `{query,mode?:hybrid\|semantic}`, список максимум 5 фрагментов |

Единственное исключение JSON Content-Type — POST K/upload с multipart/form-data. Origin и X-CSRF-Protection: 1 обязательны и здесь. Успешный 202 означает сохранение/очередь, не готовый индекс. Poll GET K для pending/processing/ready/failed и error_code; active_revision указывает рабочий индекс. Извлечённый текст доступен после extraction даже при ошибке embeddings. Оригиналы и внутренний file_key API не возвращает.

Дополнительные ошибки: file_too_large (413), empty_document/invalid_file/unsupported_file/ocr_required/unsafe_url/html_required/robots_denied (422), knowledge_limit (429), embedding_unavailable/embedding_invalid/embedding_changed/extractor_unavailable (503). Временные import/robots ошибки могут иметь 503 и повторяться worker; постоянные — 422. import_partial — предупреждение у ready web-источника. Search score: cosine для semantic и RRF для hybrid; similarity всегда cosine, это не вероятность. Полный контракт и поля — openapi.json.

## AI/preview API (этап 3)

`A=/api/v1/organizations/{id}/assistants/{assistantId}`. GET доступны членам, изменения/генерация — owner/admin. Conversation ID всегда проверяется вместе с company/assistant; IDs в JSON не принимаются. Cookie/Origin/JSON/X-CSRF-Protection: 1 сохранены и для SSE.

| Метод | Путь | Назначение |
|---|---|---|
| GET, PATCH | A/ai/settings | Настройки инструкций и configured provider metadata |
| GET, POST | A/preview/conversations | Последние 100 диалогов / создать preview |
| GET | A/preview/conversations/{conversationId} | 100 сообщений, executions/sources, next_before; query before для прошлой страницы |
| DELETE | A/preview/conversations/{conversationId} | Отменить и удалить историю |
| POST | A/preview/conversations/{conversationId}/cancel | Идемпотентная отмена активного запуска |
| POST | A/preview/conversations/{conversationId}/messages | JSON request_id/content, полный AIResult |
| POST | A/preview/conversations/{conversationId}/messages/stream | POST SSE start/delta/complete/error |

Создание — 201, JSON results/replay — 200. Stream использует fetch и HttpOnly cookie, не EventSource и не токены в URL. Ошибки до stream start — HTTP JSON; после — event:error с machine code. Полный контракт в OpenAPI. Sources — контекст запроса, не гарантированные цитаты. Mock result явно содержит provider=mock/model=deterministic-demo-v1; stats неизвестных токенов null.

Коды: ai_disabled/ai_busy/ai_unavailable (503), ai_timeout (504), ai_invalid_response/ai_incomplete (502), ai_canceled/request_in_progress/request_conflict/conversation_busy/conversation_closed (409), ai_context_limit (422). При BGE-M3 ошибке нет скрытого fallback; при нуле подходящих sources policy_fallback=true, для Ollama ответ формируется сервером без вызова модели. Same UUID/content replay не запускает новую генерацию; failed/canceled execution возвращается как сохранённый result, новая попытка требует нового request_id.

Коды ошибок: validation (422), invalid_credentials/unauthorized (401), csrf/forbidden (403), not_found (404), conflict (409), rate_limited (429, Retry-After 900), ai_not_configured (422), internal (500), not_ready (503). Login не различает отсутствующую почту и неверный пароль. Регистрация не означает подтверждение email и не имитирует отправку письма.

Минимальные payload: компания `{name,country}` (timezone по умолчанию UTC, default_locale en); ассистент `{name}` (draft, visitor, en); сайт `{name,domain,assistant_id}`. Language policy visitor/fixed — сохранённая настройка; AI имеет отдельную policy auto/fixed/default. Страна — две ASCII-буквы ISO, timezone — имя IANA, языки ru/en/cs/de. Domain — DNS-имя, а не URL, IP, путь или wildcard. Для production widget требуется отдельное подтверждение владения.

## Публичный widget, этап 4

Добавлены GET/POST/PATCH `/organizations/{id}/sites/{siteId}/widget`, POST `/widget/logo` (multipart) и `/widget/verify` (JSON), с прежними cookie/Origin/CSRF и ролями owner/admin для записи. Все пути с `/api/v1`.

Public: GET `/public/widgets/{widgetId}/config`, POST `/sessions`, GET/POST `/sessions/{sessionId}/messages`, POST `/sessions/{sessionId}/cancel`, GET `/logo`. История/отправка/отмена требуют opaque visitor Bearer, точный разрешённый Origin и проверку widget/session/expiry; cabinet cookie не используется. Public JSON writes требуют Origin/Content-Type, отдельный cabinet CSRF header им не нужен. CORS только для проверенных origins. POST messages — SSE start/delta/complete/error, тот же idempotency contract, публичная схема исключает tenant IDs/внутренние источники. Ошибки: widget_unavailable (404), widget_origin (403), visitor_unauthorized (401), widget_rate/widget_capacity/widget_session_limit (429), widget_verification/widget_logo (422), inference codes этапа 3. Полные schemas — [OpenAPI](openapi.json), threat model/limits — [widget.md](widget.md).
