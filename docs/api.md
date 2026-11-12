# REST API

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

Коды ошибок: validation (422), invalid_credentials/unauthorized (401), csrf/forbidden (403), not_found (404), conflict (409), rate_limited (429, Retry-After 900), ai_not_configured (422), internal (500), not_ready (503). Login не различает отсутствующую почту и неверный пароль. Регистрация не означает подтверждение email и не имитирует отправку письма.

Минимальные payload: компания `{name,country}` (timezone по умолчанию UTC, default_locale en); ассистент `{name}` (draft, visitor, en); сайт `{name,domain,assistant_id}`. Language policy visitor/fixed — только сохранённая настройка. Страна — две ASCII-буквы ISO, timezone — имя IANA, языки ru/en/cs/de. Domain — DNS-имя, а не URL, IP, путь или wildcard. Владение сайтом не проверяется.
