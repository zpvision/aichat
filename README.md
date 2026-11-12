# site-assistant — этап 1

Рабочий фундамент SaaS: компании, участники и роли, конфигурации ассистентов и сайтов, безопасные сессии и кабинет RU/EN/CS/DE. AI пока не подключён. Публичное название задаёт `APP_BRAND_NAME`.

Требования: Go 1.27.4+, Node.js 22+ (проверено на 24), Docker Compose. В корне сохранён исходный учебный пример GoLand; приложение запускается из `backend/`, а не через корневой `main.go`. При разрешённом GOTOOLCHAIN=auto Go сам загрузит требуемую версию из backend/go.mod.

## Локальный запуск (PowerShell)

Из корня:

```powershell
Copy-Item backend/.env.example backend/.env
docker compose up -d
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

Если БД тестов уже создана, повторять `createdb` не нужно. Без `TEST_DATABASE_URL` интеграционный тест явно пропускается. Он создаёт/удаляет только собственную случайную схему в тестовой БД, проверяет повторное применение миграций и весь API, включая изоляцию и роли.

Для браузерных тестов остановите локальный backend, в новом терминале из `backend/` установите `DATABASE_URL` на **site_assistant_test**, затем `go run ./cmd/api -migrate` и `go run ./cmd/api`. Frontend запустите как обычно. После этого в `frontend/`: `npx playwright install chromium`, `npx playwright test`. Тест оставляет свои записи только в тестовой БД; снимки 360/768/1440 px — в игнорируемом `test-results/`. Не запускайте его против production или пользовательской БД.

## Документация

- [Архитектура](docs/architecture.md), [БД](docs/database.md), [API](docs/api.md), [OpenAPI](docs/openapi.json).
- [Локализация](docs/localization.md), [развёртывание](docs/deployment.md), [следующие этапы](docs/roadmap.md).
- [Правила для следующих сессий](AGENTS.md).
