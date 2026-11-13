# Публичный чат сайта — этап 4

## Устройство

`widget/` — отдельный TypeScript/React/Vite клиент. Go обслуживает собранные файлы из `WIDGET_DIST_DIR`. `loader.js` — самостоятельный classic IIFE без React и runtime imports (примерно 4.4 KB, 2 KB gzip). При установке выполняется один запрос конфигурации. Кнопка живёт в Shadow DOM; основной React bundle и iframe создаются только при первом открытии. Пока чат не используется, нет polling/WebSocket, сторонних SDK, аналитики или шрифтов CDN.

```html
<script src="https://YOUR_WIDGET_DOMAIN/widget/loader.js" data-widget-id="PUBLIC_WIDGET_ID" async></script>
```

Код копируется в кабинете: компания → «Сайты» → «Внешний вид чата». Подходит для HTML и поля пользовательского JavaScript в CMS/WordPress. CSP самого сайта должна разрешать scripts/styles/connect/frame с домена виджета. Loader загружает собственный `loader.css` в Shadow DOM; iframe полностью изолирует CSS диалога. Не встраивайте виджет на одной origin с недоверенным сайтом: production widget должен иметь отдельный доверенный origin.

Go выдаёт iframe `/widget/{widgetId}?parent_origin=...&channel=...` с CSP `frame-ancestors` из точного allowlist. Токен посетителя в URL отсутствует. Sandbox разрешает только scripts и same-origin; формы не навигируют, отправка выполняется fetch. `postMessage` проверяет одновременно `event.source`, точный `event.origin` и случайный channel UUID с обеих сторон. `*` как targetOrigin не используется. Чужие сообщения игнорируются. Закрытие скрывает iframe, отменяет генерацию и сохраняет беседу; повторное открытие восстанавливает историю. На мобильном iframe занимает visualViewport, учитывая его resize/offset и safe areas. Реальная экранная клавиатура отдельного телефона требует проверки на устройстве.

## Настройки

Один виджет принадлежит одному site, который уже связан с assistant и organization. Публичный UUID не секрет. Он никогда не даёт права кабинета и не принимает клиентские organization/assistant IDs. Новый виджет отключён.

Форма сохраняет в PostgreSQL заголовок/подзаголовок/приветствие отдельно для RU/EN/CS/DE, четыре цвета, позицию, отступы, видимость кнопки, сохранение сессии, разрешение нового разговора, default/browser locale и HTTPS ссылку на политику конфиденциальности. Пустой перевод использует default locale, затем встроенный словарь. Язык интерфейса независим от AI language policy. Live preview использует тот же iframe/UI, но не создаёт сессий и не отправляет сообщения.

Логотип: PNG/JPEG до 256 KiB и 512×512, сервер декодирует и заново кодирует PNG, исключая SVG, HTML и метаданные. Файлы — случайные ключи под `WIDGET_FILE_ROOT`; это публичный брендинг, а не оригиналы документов базы знаний. Заменённый логотип удаляется. После удаления сайта локальный orphan logo может потребовать уборки; автоматического GC пока нет.

## Домен и Origin

В allowlist — до 10 точных origins. HTTPS домен сайта и явно указанные поддомены, без wildcard, чужих доменов и произвольных портов. Нормализация IDNA, регистра, trailing dot и стандартных портов выполняется сервером. Localhost/loopback с портами разрешаются только при явном `WIDGET_ALLOW_LOCALHOST=true` в dev/test. Production запрещает этот флаг и требует HTTPS widget URL.

Для production необходимо положить выдаваемый кабинетом challenge в `https://DOMAIN/.well-known/site-assistant-verification.txt`, затем нажать проверку. Используется существующий DNS-pinned SSRF-safe importer: публичные IP, bounded redirects/response/time, без proxy, каждый redirect проверяется. Проверяется точное содержимое challenge. До `verified_at` включить production widget нельзя. Базовый legacy `sites.status=unverified` не используется как разрешение; актуальное подтверждение хранится в `widgets.verified_at`. Автоматической периодической перепроверки владения пока нет.

Loader запрашивает config с браузерным Origin сайта. Запросы из iframe идут на origin виджета с `X-Widget-Origin` родителя. Сервер проверяет Origin виджета и allowlist родителя; для same-origin GET без Origin дополнительно нужен браузерный `Sec-Fetch-Site: same-origin`. POST всегда требует Origin и JSON. CORS возвращает только точный разрешённый Origin, без credentials и wildcard. CSP и handshake подтверждают реальное браузерное встраивание; Origin не является секретом и может быть подделан серверным клиентом, поэтому бюджетирование обязательно.

## Анонимные сессии и общий AI

Первое сообщение создаёт visitor session и существующую `conversations` запись `kind=website`. Случайный токен 32 bytes (43 base64url символа) возвращается один раз; PostgreSQL хранит SHA-256 hash. Каждая история/отправка/отмена проверяет widget + session + token + origin + expiry + enabled. UUID сессии без токена бесполезен. Все связи tenant/assistant/site обеспечены составными FK и SQL scopes.

Loader хранит текущую пару ID/token в `sessionStorage` родительской страницы (если разрешено компанией), передаёт её iframe только по проверенному bridge. В iframe токен находится в памяти. Authorization Bearer, `credentials: omit`; зависимости от third-party cookies нет. Токен не помещается в URL, логи или localStorage. Исключение из запрета хранения cabinet auth tokens относится только к этому visitor session token. XSS самого клиентского сайта способен прочитать его sessionStorage; политика безопасности сайта остаётся ответственностью владельца.

Сессия истекает через 4 часа (настраивается 1–24). «Новый диалог» с подтверждением забывает старый token и лениво создаёт новую сессию; старые записи остаются в PostgreSQL. Это не постоянная идентификация посетителя и не восстановление между устройствами. Автоматическое удаление истёкших историй и retention policy нужны до production. Удаление сайта каскадно удаляет его widget, visitor sessions и website conversations/messages/executions; кабинет предупреждает об этом.

Публичный адаптер вызывает тот же `ai.Service.generate`, что preview. Отличаются проверка доступа и лимиты канала. Retrieval переиспользует существующий knowledge search; внутренний `Retrieve` вызывается только после visitor authorization и не опубликован отдельным API. В `ai_executions` сохраняются реальные переданные источники/версии, но публичный ответ не раскрывает RAG chunks, tenant IDs или внутреннюю диагностику. Идемпотентный request UUID и DB-блокировки не позволяют двум запросам создать два ответа одного диалога. NDJSON provider → общий SSE emitter → публичные start/delta/complete/error. Незавершённый текст не сохраняется как completed. Обрыв/закрытие/Stop отменяют context; отдельный cancel endpoint работает и между процессами через DB-состояние. Отключение/expiry наблюдаются активной генерацией в пределах примерно секунды.

Текст выводится как React text nodes с безопасными HTTP(S) ссылками, без HTML/Markdown execution. Абзацы, списки и Unicode сохраняются как plain text. Никаких tool calls/HTTP действий по указаниям RAG не выполняется.

## API

Префикс всех методов — `/api/v1` (полные schemas в [OpenAPI](openapi.json)).

| Метод | Путь | Доступ |
|---|---|---|
| GET / POST / PATCH | `/organizations/{id}/sites/{siteId}/widget` | member read; owner/admin write |
| POST | предыдущий путь + `/logo` | owner/admin, multipart file |
| POST | предыдущий путь + `/verify` | owner/admin, JSON `{}` |
| GET | `/public/widgets/{widgetId}/config` | enabled + exact origin |
| POST | `/public/widgets/{widgetId}/sessions` | enabled + origin, JSON `{}` |
| GET / POST | `/public/widgets/{widgetId}/sessions/{sessionId}/messages` | visitor Bearer + origin |
| POST | предыдущий session path + `/cancel` | visitor Bearer + origin |
| GET | `/public/widgets/{widgetId}/logo` | sanitized public branding |

POST messages body: `{"request_id":"UUID","content":"Question"}`. SSE: start `{request_id}`, delta `{text}`, complete `{request_id,messages}`, error `{code}`. Ошибки до streaming — обычный JSON. Частичные/отменённые ответы не становятся завершёнными. Cabinet cookies не дают публичной visitor authority, visitor token не даёт cabinet authority.

## Ограничения ресурсов

| Уровень | Начальный лимит |
|---|---|
| Сообщение / история AI | 2000 Unicode chars / существующее bounded history/context окно AI |
| Сессия | 50 сообщений, история максимум 100 user/assistant messages |
| Сайт / организация | 300 / 1000 генераций в rolling hour, 2 / 4 одновременно |
| Создание сессий PostgreSQL | 500/site, 2000/org в rolling 24 hours |
| Fast limiter | 180 requests/peer IP/min, 5 sessions/site/IP/min, 10 messages/token/min |
| Inference | общий AI_MAX_CONCURRENT, timeout и cancellation этапа 3 |

Бюджеты PostgreSQL атомарны между процессами, не сбрасываются перезапуском. Повтор completed request не расходует message budget повторно, но проходит fast limiter. Ошибки после начала исполнения учитываются, чтобы нельзя было бесплатно перегружать inference. Fast limiter bounded (10k keys) хранится в памяти одного процесса; для нескольких экземпляров и reverse proxy нужен доверенный edge limiter/общий Redis limiter. Сейчас X-Forwarded-For игнорируется, чтобы его нельзя было подделать; reverse proxy peer IP будет общим. Публичный anti-bot/billing не реализован.

## Локальный запуск и проверка

1. Запустить зависимости этапов 1–3 по README. Нужны PostgreSQL/pgvector и локальная BGE-M3 для RAG. Gemma не требуется.
2. В `widget/`: `npm ci`, `npm run build`. Go будет отдавать production assets из `../widget/dist`.
3. Backend env: `WIDGET_PUBLIC_URL=http://localhost:8080`, `WIDGET_ALLOW_LOCALHOST=true`, `AI_PROVIDER=mock`; миграции `go run ./cmd/api -migrate`, запуск `go run ./cmd/api`. Существующий `.env` обновлять осознанно; `scripts/backend.ps1` загружает его перед запуском.
4. Запустить frontend как раньше (5173), `npm run dev` в `widget/` (4173). В кабинете создать/выбрать site, «Внешний вид чата», создать widget, добавить `http://localhost:4173` в allowed origins и включить.
5. Открыть `http://localhost:4173/widget/demo/index.html?widget=PUBLIC_WIDGET_ID`. Это независимый HTML с собственным CSS, не кабинет. Loader вызывается реальным script tag. Порт4173 служит только demo/dev: iframe/API/assets обслуживает Go8080. Для изменения UI пересобрать widget.
6. `npm test` и `npm run build` в обоих клиентах; Playwright в frontend с API на существующей `_test` БД, `WIDGET_ALLOW_LOCALHOST=true`, `AI_PROVIDER=mock`, BGE-M3 доступна. `npx playwright test`. Полный порядок — [testing.md](testing.md).

Screenshots создаются в игнорируемом `frontend/test-results/widget-360.png`, `widget-768.png`, `widget-1440.png`; отчётные копии — `artifacts/stage4/`. Их не добавляем в Git согласно AGENTS.md. Browser scenario использует настоящий BGE-M3 retrieval и демонстрационную генерацию; качество Gemma не оценивалось.

## После возвращения GPU

На этом этапе не выполняются обращения к старому GPU/FinKoper, установка Gemma или production deployment. Сохранить `OLLAMA_BASE_URL` embeddings; для генерации отдельно настроить региональные `AI_PROVIDER=ollama`, `AI_BASE_URL`, `AI_MODEL` и параметры из [ai-engine.md](ai-engine.md). Затем отдельно проверить совместимость модели, streaming, отмену, нагрузку и качество RU/EN/CS/DE. Mock запрещён в production; disabled показывает недоступность, недоступный Ollama возвращает безопасную ошибку. Доменные/CORS лимиты не заменяют production нагрузочные/безопасностные испытания.
