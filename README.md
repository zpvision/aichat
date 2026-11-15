# site-assistant — этапы 1–8

Stage 9A.3: компания → **Аналитика / Analytics** (Owner/Admin). Рабочие PostgreSQL KPI, динамика публичных диалогов и заявок, качество AI, команда, источники и реальные квоты. RU/EN/CS/DE, Cosmic Violet, фильтры календарных дат/часового пояса/сайта/ассистента. Применить новую индексную миграцию011 обычным мигратором перед запуском API. [Определения, формулы, ограничения и проверки](docs/analytics.md). Без утверждений об успешном решении обращений или качестве Gemma.

Stage 9A.2: публичный лендинг **Cosmic Violet** на `/`, регистрация `/register`, вход `/login`, прежний кабинет `/app`. Четыре языка, сценарное демо без создания лидов/расхода квот, оригинальные SVG-композиции и изолированные стили. Кабинет, клиентский виджет и S3 не перекрашены. [Архитектура, визуальные проверки и ограничения](docs/marketing-design.md).

Stage 9A.1: private S3-compatible original/logo storage, mixed local/S3 registry, authorized original download, safe administrative migration/export and real Yandex tests. Apply migration010 before API startup. Default remains STORAGE_PROVIDER=local; configure protected S3_* environment to select s3. [Object storage, migration and verification](docs/object-storage.md). Never overwrite an existing .env with the example or start bulk migration automatically.

Stage 9A operational preparation: [readiness and remaining risks](docs/production-readiness.md), [backup/restore](docs/backup-restore.md), [retention](docs/retention.md), [local load tests](docs/load-testing.md), [independent RU/EU deployment](docs/deployment-ru-eu.md). Production/GPU deployment is not performed. Operations CLI requires the existing deployment administrative credential; defaults to read-only diagnostics/dry-run. No old migrations or root GoLand files are changed.

Stages 1–8: modular Go SaaS, multilingual cabinet, knowledge/RAG, preview and website chat, operator inbox, lead collection/mini-CRM, universal company API with signed outgoing webhooks, regional plans/subscriptions and PostgreSQL usage quotas. FinKoper and other CRMs consume AI Chat data; no destination-specific client or SSO exists. Gemma is not connected or quality-tested. Public brand: APP_BRAND_NAME. See docs/finkoper-integration.md for the integration contract.

Stage 8: apply the appended migration with `./scripts/backend.ps1 -Migrate`, then restart the API. `BILLING_TRIAL_DAYS=14` is configurable (1–90); existing and new organizations receive Trial. Owner/admin: company → **Тариф и использование / Plan and usage**. Prices/payments are not configured. Pilot Start/Business grants use only the protected deployment CLI (`backend/cmd/subscription`), never a public endpoint. AI limits do not block manual operator chat. [Plans, quotas, storage accounting and CLI instructions](docs/billing.md).

Требования: Go 1.27.4+, Node.js 22+ (проверено на 24), Docker Compose. В корне сохранён исходный учебный пример GoLand; приложение запускается из `backend/`, а не через корневой `main.go`. При разрешённом GOTOOLCHAIN=auto Go сам загрузит требуемую версию из backend/go.mod.

## Локальный запуск (PowerShell)

Этап 5: применить `005_operator_inbox.sql` обычным мигратором, пересобрать widget и открыть «Диалоги». «Сотрудники» позволяет создать одноразовую ссылку приглашения без email. AI/человек используют общую историю, PostgreSQL fencing и обновления по курсору. [Рабочее место операторов, права и проверки гонок](docs/operator-inbox.md). Настоящая Gemma по-прежнему не подключена.

Для публичного чата дополнительно собрать `widget/` (`npm ci`, `npm run build`) перед запуском API. В существующий backend `.env` добавить параметры `WIDGET_*` из `.env.example`; для localhost нужен явный `WIDGET_ALLOW_LOCALHOST=true`. Независимая demo HTML: в `widget/` выполнить `npm run dev`, открыть `http://localhost:4173/widget/demo/index.html?widget=PUBLIC_WIDGET_ID`. ID, код установки и настройки — компания → «Внешний вид чата». Разрешить origin `http://localhost:4173` и включить widget. Полный порядок, анонимные сессии, безопасность, настройки и ограничения: [docs/widget.md](docs/widget.md).

Из корня:

```powershell
Copy-Item backend/.env.example backend/.env
docker compose build postgres extractor
docker compose up -d postgres
ollama pull bge-m3
./scripts/backend.ps1 -Migrate
./scripts/backend.ps1
```

В другом терминале:

```powershell
cd frontend
npm ci
npm run dev
```

Открыть **http://localhost:5173** (именно localhost, как в `PUBLIC_APP_URL`). Backend: http://localhost:8080/healthz. Регистрация даёт одинаковый ответ для нового/существующего адреса; затем войдите с паролем. Подтверждения email пока нет.

Ollama устанавливается локально и запускается через `ollama serve`, если служба ещё не работает. Без Ollama кабинет/API продолжают работать, но индексация и поиск показывают явную ошибку. PDF использует одноразовый ограниченный контейнер extractor, не отдельный API. GPU не обязателен. **При обновлении с этапа 1 не перезаписывайте существующий backend/.env** — добавьте нужные параметры из примера. Volume PostgreSQL сохраняется; `docker compose up` пересоздаст только контейнер с новым образом PostgreSQL 17 Alpine + pgvector.

Этап 3: применить новую миграцию той же командой `./scripts/backend.ps1 -Migrate`, в кабинете открыть «Тестовый чат» у ассистента. AI_PROVIDER=mock по умолчанию не обращается к Gemma; в production этот режим запрещён (явно выбрать disabled/ollama). AI_BASE_URL генерации независим от OLLAMA_BASE_URL embeddings. Старый GPU-сервер не использовать и Gemma локально не скачивать. Конфигурация/будущее подключение — [AI engine](docs/ai-engine.md).

Linux/macOS: `cp backend/.env.example backend/.env`; в каталоге backend выполнить `set -a; . ./.env; set +a`, затем `go run ./cmd/api -migrate` и `go run ./cmd/api`. Остальные команды аналогичны. Миграции запускаются явно перед API.

Остановка PostgreSQL без удаления данных: `docker compose stop`. Не используйте `down -v` для сохранения данных. При конфликте подсети скопируйте корневой `.env.example` в `.env` и задайте свободный `DOCKER_SUBNET`. Чужие Docker-сети удалять не нужно.

## Проверки

```powershell
docker compose exec -T postgres createdb -U site_assistant site_assistant_test
cd backend
$env:TEST_DATABASE_URL='postgres://site_assistant:local-development-only@localhost:5432/site_assistant_test?sslmode=disable'
go test ./... -count=1 -v
go vet ./...
go build ./cmd/api
cd ../frontend
npm test
npm run build
npm audit
```

Полные RAG-проверки: перед `go test` дополнительно установить `PDF_TESTS=1`, `PUBLIC_IMPORT_TESTS=1`, `REAL_EMBEDDINGS=1`; необязательно `RAG_REPORT_PATH` с абсолютным путём отчёта. Требуются extractor и настоящая BGE-M3. Без этих флагов соответствующие реальные тесты явно пропускаются. [Инструкция и ограничения RAG](docs/rag.md), [фактические метрики](docs/rag-evaluation.json).

Если БД тестов уже создана, повторять `createdb` не нужно. Без `TEST_DATABASE_URL` интеграционный тест явно пропускается. Он создаёт/удаляет только собственную случайную схему в тестовой БД, проверяет повторное применение миграций и весь API, включая изоляцию и роли.

Для браузерных тестов остановите локальный backend, в новом терминале из `backend/` установите `DATABASE_URL` на **site_assistant_test**, затем `go run ./cmd/api -migrate` и `go run ./cmd/api`. Frontend запустите как обычно. После этого в `frontend/`: `npx playwright install chromium`, `npx playwright test`. Тест оставляет свои записи только в тестовой БД; снимки 360/768/1440 px — в игнорируемом `test-results/`. Не запускайте его против production или пользовательской БД.

## Документация

- [Архитектура](docs/architecture.md), [БД](docs/database.md), [API](docs/api.md), [OpenAPI](docs/openapi.json).
- [Локализация](docs/localization.md), [развёртывание](docs/deployment.md), [следующие этапы](docs/roadmap.md).
- [Правила для следующих сессий](AGENTS.md).
- [База знаний/RAG](docs/rag.md), [проверки обоих этапов](docs/testing.md).
- [AI-движок и тестовый чат](docs/ai-engine.md). Для browser suite использовать существующую site_assistant_test, AI_PROVIDER=mock и отдельный KNOWLEDGE_FILE_ROOT=./data/knowledge-test. Mock generation не заменяет реальный BGE-M3 retrieval.

## Stage 6: scenarios and leads

Apply the appended migration 006 with `./scripts/backend.ps1 -Migrate`. Build both clients as above. Open Scenarios, select the assistant, enable collection and configure fields; set a privacy URL in Website chat settings. Use Start a request in the installed widget, then review and explicitly send. Leads contains assignments, statuses, notes and links to existing conversations. No new dependencies or GPU are required. See [docs/leads.md](docs/leads.md) for mock fixtures, validation, permissions, API and retention limits.

## Stage 7: universal integrations

Apply migrations 007 and 008 with `./scripts/backend.ps1 -Migrate`. In company ? Integrations, create a read-only API key (optionally grant creation), configure a webhook, copy its signing secret once, and inspect test deliveries. FinKoper and all other CRMs are consumers of AI Chat, not destinations with invented APIs. See [integration contract and local receiver](docs/finkoper-integration.md). Set a stable base64-encoded 32-byte `WEBHOOK_ENCRYPTION_KEY` to enable webhook creation/delivery. Without it, cabinet and API keys still work. `WEBHOOK_DEV_RECEIVER_URL` optionally permits one exact loopback URL in development and is forbidden in production. Example receiver: `node scripts/webhook-receiver.mjs`, with WEBHOOK_SIGNING_SECRET set to the one-time endpoint secret. No FinKoper deployment, SSO or GPU is involved.
