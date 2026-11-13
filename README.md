# site-assistant — этапы 1–5

Фундамент SaaS, база знаний, общий AI-движок и публичный чат сайта: компании, роли, ассистенты, документы/импорт, hybrid retrieval, инструкции, preview и website диалоги с историей, streaming и отменой. Кабинет и iframe-виджет RU/EN/CS/DE. Embeddings — настоящая Ollama BGE-M3. Генерация по умолчанию — **явный MockProvider**, настоящая Gemma не подключена и не проверена. Публичное название задаёт `APP_BRAND_NAME`.

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
