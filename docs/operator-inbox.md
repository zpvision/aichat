# Рабочее место операторов — этап 5

## Границы и запуск

`internal/inbox` расширяет тот же Go-монолит и существующие conversations/messages/AI executions. Новых backend, очередей и систем хранения переписки нет. Очередь операторов содержит только `kind=website`: preview доступен только owner/admin через прежний API и не попадает в inbox. Gemma, FinKoper, CRM, лиды, SLA и внешние уведомления не подключены.

Применить append-only `005_operator_inbox.sql` через `./scripts/backend.ps1 -Migrate`, собрать `widget/` (`npm ci`, `npm run build`), запустить backend/frontend/demo как в [widget.md](widget.md). В компании появились «Диалоги» и «Сотрудники». Виджет устанавливается прежним loader; обновлённые assets обслуживает Go. Для dev/test — AI_PROVIDER=mock с явной маркировкой; production mock запрещён. При disabled посетитель может оставить сообщение и запросить сотрудника, а сотрудник — вести ручной разговор. Embeddings и generation URL остаются независимыми региональными настройками.

При обновлении необходимо остановить старые экземпляры API: бинарник этапа 4 не знает fencing/режимов и не должен работать параллельно с этапом 5. Миграция сохраняет существующие данные; mixed-version rolling upgrade здесь не проверялся.

## Состояния

`conversations.status` open/closed сохранён для совместимости. Отдельный `control_mode` и монотонный `state_version` определяют право отвечать; это не статус конкретной генерации. Дополнительно сохраняются assigned_user_id, last_message_at, last_visitor_at, requested_at, closed_at. Сайт/ассистент/компания разрешаются сервером.

| Действие | Условие | Результат |
|---|---|---|
| Посетитель просит сотрудника | ai_active | human_requested, AI отменяется |
| Забрать | открытый и свободный либо уже свой | human_active, назначение текущему сотруднику |
| Назначить | owner/admin, участник той же компании | human_active; пустое назначение → human_requested |
| Вернуть AI | открытый, human_requested/human_active | ai_active, назначение снимается |
| Закрыть | открытый | closed, без автоматического открытия |
| Заметка | открытый, разрешённый сотруднику | приватное событие, без смены режима |

Из любого открытого режима можно закрыть разговор. Через «Забрать» нельзя отнять уже назначенный другому диалог: owner/admin используют явное назначение. Повторные переходы требуют того же request_id/body для идемпотентного результата; случайный устаревший version возвращает state_conflict (409). После закрытия visitor history доступна до expiry; новое сообщение отклоняется. Новый разговор создаётся явно кнопкой виджета, если разрешён компанией. Возврат AI сам по себе не генерирует ответ: следующий вопрос запускает прежний pipeline.

## Перехват и отсутствие двойного управления

PostgreSQL — источник истины. Все begin/ручные сообщения/переходы/успешный Finish блокируют одну строку conversations `FOR UPDATE`. При начале AI execution сохраняет снимок state_version. Перехват атомарно меняет режим, повышает версию, назначает сотрудника, переводит processing execution и незавершённое assistant message в canceled с пустым content, добавляет событие. После commit сигнал отмены отправляется локальному context; другой процесс заметит persisted cancellation фоновым монитором.

Одной проверки версии перед network write недостаточно. Каждый website start/delta и complete проходит `ai.Store.Deliver`: транзакция удерживает `FOR SHARE` этой же строки **во время сериализации и flush**. Перед отправкой проверяются режим, версия и статус execution. Handoff ждёт уже начатый flush; после подтверждённого commit следующая старая отправка не пройдёт проверку. Flush ограничен write deadline 10 секунд: медленный клиент способен задержать подтверждение перехвата в пределах этого ограничения. Уже отправленные до commit сетевые байты отозвать невозможно; виджет при получении нового режима отменяет поток и сверяет сохранённую историю.

`Finish(completed)` повторно проверяет mode/version под эксклюзивной блокировкой в той же транзакции, где сохраняется ответ. Провайдер, игнорирующий cancel, не сможет опубликовать старый результат. Ручной ответ и resume/close сериализованы той же строкой: либо ответ сохранён до перехода, либо отклонён после него. Два take с одинаковой исходной версией дают одного победителя. Решение работает между процессами без Redis; память active map ускоряет отмену, но не даёт полномочия завершить execution.

Visitor receive использует существующий AI begin: под блокировкой выбирает AI либо ручное сохранение сообщения. В human_requested/human_active/disabled не создаются AI execution и фиктивный assistant reply. Идемпотентный request UUID сохраняется в прежних conversation_messages; повтор body не создаёт дубль, изменённый body конфликтует. Ручной путь не занимает GPU slots/concurrency budget. Session/site/org лимиты посетителя продолжают действовать; hourly budget теперь считает visitor messages в обоих режимах.

После crash незавершённый execution сохраняется processing до прежней expires_at (AI timeout + 15 секунд), затем общий recovery переводит его в failed/ai_interrupted. Recovery также сначала блокирует conversation, сохраняя единый порядок блокировок. Нельзя мгновенно считать все processing записи брошенными при старте одного процесса: другой экземпляр Go может ещё обрабатывать их.

## История и данные

Прежняя `conversation_messages` поддерживает role=operator, nullable execution_id, author_user_id и request_id. Составные tenant/conversation/member FK остаются обязательными. operator content при построении AI history преобразуется в assistant message; завершённые пользовательские сообщения сохраняются в контексте, а отменённые/failed AI ответы исключаются. Заметки вообще не являются messages и никогда не попадают в prompt.

`conversation_events` хранит actor/target, timestamp, kind и idempotency hash. Note — приватное событие с текстом, assignment — приватные метаданные; публичный DTO выдаёт только безопасные служебные mode events без сотрудников/заметок. Public DTO не содержит organization/assistant/conversation IDs, member IDs, execution context или токены. В правой колонке кабинета доступны заметки и история передачи.

`conversation_reads` хранит last_sequence отдельно для каждого участника. Read монотонен и ограничен существующими сообщениями. Непрочитанные считаются только по visitor messages. Прочтение одним сотрудником не очищает счётчик другого. Отметка делается для открытой видимой переписки; hidden вкладки не опрашиваются.

## Обновления, фильтры и интерфейс

Polling каждые 3 секунды, без Redis/PubSub/WebSocket. Список и summary загружаются ограниченно; история — по change cursor. Messages имеют стабильный sequence для порядка и change_seq из общего sequence для обновлений, в том числе completed/canceled. Mode/note events имеют тот же change sequence. Writers сериализованы блокировкой conversation; читатель берёт shared lock для согласованного cursor snapshot. Таким образом, изменение статуса уже видимого сообщения не теряется после начальной загрузки.

Начальная страница — последние 50 изменений; `after` — до 100 следующих, `before` — более старые. Клиенты объединяют записи по ID, сообщения сортируют по исходному message sequence. `more`/next_before позволяют загрузить старые порции. Полная история всех разговоров не скачивается. Виджет опрашивает только открытый и видимый iframe с действующей сессией; без сессии и при закрытии опроса нет. Restore/reload использует прежний parent sessionStorage token и persisted mode. После потери сети следующий успешный cursor poll восполняет изменения.

Список: keyset pagination (30 по умолчанию, максимум 50), updated_at/id, фильтры mode/site/assistant/assigned/me/date и literal substring по completed message content. Даты фильтруются по UTC включительно; формат отображения — locale пользователя. PostgreSQL pg_trgm индекс поддерживает substring search без отдельного поискового сервера. Статистика unread/requested ограничена доступной текущему участнику областью. Ненавязчивая общая полоса и badge обновляются в кабинете; всплывающие окна, push и обещания времени ответа отсутствуют.

Desktop — список, переписка и информация; на ширинах до 850px — отдельный список и открытый разговор с кнопкой возврата, информация ниже истории. При переходах/сетевых ошибках UI показывает машинные ошибки в четырёх словарях. Staff reply сохраняет idempotency key при повторе после сетевого сбоя; при смене выбранного диалога устаревшие async responses не меняют новую переписку. Текст и заметки отображаются без выполнения HTML.

## Роли и приглашения

Owner/admin видят все website разговоры компании, могут отвечать в human_active, перехватывать, назначать, возвращать AI и закрывать. Operator видит свои (в том числе закрытые) и свободные открытые; может забрать свободный, отвечать/делать заметки/возвращать AI/закрывать только свой. Чужое назначение запрещает и прямой GET истории, и запись. Изменение знаний/системных настроек не разрешено. Проверки назначения повторяются в транзакции мутации, а не только в React.

Owner/admin создают invitation с ролью admin/operator. Случайный 256-bit token хранится только SHA-256, действует 7 дней, одноразовый; максимум 50 действующих приглашений. Ссылка возвращается один раз, в URL fragment `/#invite=...`, чтобы секрет не уходил HTTP access logs/referrer. Кабинет немедленно удаляет fragment и держит token в памяти до явного принятия; после reload ссылку нужно открыть снова. Никаких писем автоматически не отправляется. После login/register кнопка принятия добавляет membership. Повтор того же пользователя идемпотентен, другого запрещён. Существующая роль никогда не повышается/понижается принятием приглашения; owner так создать нельзя. Можно отозвать неиспользованное приглашение. Управление отделами, удаление участников и редактирование ролей не входят в этап.

## API и проверка

Все cabinet методы — прежние cookie + Origin + X-CSRF-Protection, публичные — прежние visitor bearer + exact origin. Контракт и payload — [OpenAPI](openapi.json).

| Группа | Методы |
|---|---|
| `/api/v1/organizations/{id}/conversations` | GET list |
| `.../{conversationId}` | GET detail; GET/POST messages; POST take/assign/resume-ai/close/notes/read |
| `/api/v1/organizations/{id}` | GET inbox-summary/members; GET/POST invitations; DELETE invitations/{invitationId} |
| `/api/v1/invitations/accept` | POST token (authenticated) |
| `/api/v1/public/widgets/{widgetId}/sessions/{sessionId}` | GET messages with after/before; POST request-human; прежние messages/cancel |

Action body `{request_id,version}`; assign добавляет assignee, notes/reply — content. Reply максимум 2000 Unicode characters, note 4000. Read `{sequence}`. Ошибки state_conflict/conversation_claimed/request_conflict/closed — 409; forbidden — 403; invitation_invalid — 422; invitation_limit — 429. Публичная ручная отправка использует прежний SSE complete с visitor message, без пустого assistant message.

Тесты PostgreSQL: `TEST_DATABASE_URL` существующей `_test` БД, `go test ./...`; отдельные случайные schema с двукратным мигратором. Для RAG/PDF/regression — флаги из [testing.md](testing.md). Browser: сначала старые 5 сценариев, затем после перезапуска собственного локального API `npx playwright test inbox.spec.ts` (новый сценарий регистрирует трёх участников и упирается в общий auth limiter при одном общем прогоне). Нельзя отключать production auth rate limit ради тестов.

Реальный crash/restart smoke: в backend `go build ./cmd/api`, затем с TEST_DATABASE_URL из `_test` выполнить `node ../scripts/test-inbox-restart.mjs`. Скрипт использует только собственный дочерний API на 18085, не останавливает чужие процессы, дважды перезапускает его, проверяет persisted interrupted generation, assignment/history и manual message при disabled. Создаёт только собственные fixture записи в тестовой БД. Нужна локальная BGE-M3, Gemma не вызывается.

## Проверенные ограничения

Проверены PostgreSQL races, browser с двумя операторами и независимым iframe, повторное подключение, четыре языка и 360/768/1440. Настоящая Gemma/GPU и её качество не проверены. Нет presence, SLA, отделов, auto-routing, внешних уведомлений или CRM. Polling даёт обычную задержку до 3 секунд; при большом backlog нужны несколько cursor pages. Fast limiter остаётся однопроцессным; production требует edge/shared limiter, нагрузочных испытаний, retention/GC и отдельной проверки настоящих мобильных устройств/Safari/Firefox. Источник страницы обращения в stage 4 не собирался; показан связанный сайт. Этап 6 не начат.
