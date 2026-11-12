# База знаний и RAG — этап 2

Реализован retrieval, без генерации ответов и без Gemma. Модуль `backend/internal/knowledge` использует существующие auth/tenant проверки, pgx pool и HTTP адаптер. Кабинет: «База знаний» → ассистент → статьи/документы/импорт сайтов/проверка знаний. Оператор читает и ищет, owner/admin изменяют знания.

## Запуск

```powershell
docker compose build postgres extractor
docker compose up -d postgres
ollama pull bge-m3
./scripts/backend.ps1 -Migrate
./scripts/backend.ps1
# отдельный терминал
cd frontend
npm ci
npm run dev
```

Ollama должна слушать `http://localhost:11434` (если не запущена — `ollama serve` в отдельном терминале). GPU не обязателен, CPU медленнее. Используются POST /api/embed и GET /api/tags; генеративные endpoints не вызываются. Основное API и кабинет стартуют без Ollama: материалы сохраняются, фоновые попытки получают явную ошибку и затем доступны для повторной индексации. Поиск без доступного embedding-сервиса возвращает 503; случайных векторов и скрытой подмены поиска нет.

При обновлении с этапа 1 volume `site-assistant_postgres_data` остаётся тем же. Образ основан на прежнем postgres:17-alpine и добавляет pgvector 0.8.7, без смены major/libc. Перед локальным обновлением сделан pg_dumpall в игнорируемый `tmp/stage1-backup.sql`. Никогда не делать `down -v`. Расширение vector требует привилегий мигратора; приложение не создаёт расширение на каждый запрос.

## Материалы и обработка

Статья: title/content/language/enabled, редактирование запускает новую revision. Язык документа задаётся независимо от языка кабинета. Если язык неизвестен, хранится пустая строка; автоматического определения языка TXT/PDF/DOCX нет, HTML использует lang при поддерживаемом значении.

Документы: UTF-8 TXT/MD; DOCX — стандартный ZIP/XML streaming, текст абзацев и таблиц в порядке документа; PDF — Poppler с разделением страниц по form feed. PDF без текстового слоя возвращает ocr_required, OCR не выполняется. Оригиналы — файлы с UUID-ключами и правами 0600 под KNOWLEDGE_FILE_ROOT, не бинарные поля БД. API отдаёт извлечённый текст, endpoint скачивания оригинала отсутствует. Интерфейс FileStorage подготовлен для будущей замены local storage.

Лимиты: файл по умолчанию 10 MiB, одновременно не более двух upload-запросов на процесс; DOCX до 1000 ZIP-entries и 20 MiB суммарного распакованного содержимого; extracted text до 2 MiB; PDF до 500 страниц (число проверяется pdfinfo до извлечения), превышение отклоняется. PDF выполняется в одноразовом контейнере с 256 MiB RAM, одним CPU, 32 PID, read-only root, tmpfs 16 MiB, без сети/capabilities и с общим таймаутом извлечения 30 секунд. Только stdin/stdout, исходное имя никогда не становится командой или путём. Размер вывода ограничен. DOCX paths не распаковываются на диск; подозрительные имена отклоняются. Подмена сигнатуры/расширения, некорректные и пустые файлы отклоняются или переходят в failed с понятным кодом.

Дубликат документа определяется SHA-256 исходных байтов внутри одного ассистента, возвращает 409 и удаляет временно сохранённую копию. У статей checksum содержимого, у веб-страниц — извлечённого текста. Извлечённый текст сохраняется до вызова embeddings, поэтому доступен для просмотра даже при ошибке Ollama. Копии с другим бинарным содержимым считаются отдельными документами. Сбой процесса между записью файла и транзакцией может оставить orphan file; автоматического удаления сирот пока нет.

## Импорт сайтов

URL нормализуется (без fragment), только HTTP/HTTPS, порты 80/443, без credentials. DNS разрешается непосредственно при Dial; **все** полученные адреса должны быть публичными, соединение открывается с проверенным конкретным IP. HTTP proxy из окружения не используется. Запрещены private/loopback/link-local/CGNAT/metadata/зарезервированные диапазоны и опасные IPv6 tunnels/NAT64. Каждый редирект повторяет защиту; для HTML редирект ограничен тем же hostname и проверяется robots.txt целевого пути до запроса.

robots.txt 404 означает отсутствие правил; остальные ошибки — закрытый отказ. Учитываются Disallow/Allow и Crawl-delay (задержка более 5 секунд приводит к отказу для этого MVP). User-Agent SiteAssistantBot/1.0. Четыре redirect hops, timeout запроса 20 секунд, HTML до 2 MiB, robots до 512 KiB. Из HTML удаляются script/style/nav/header/footer/aside/form и прочее; предпочтение main/article, заголовок и lang сохраняются. JavaScript не выполняется. React показывает plain text, HTML не вставляется через innerHTML.

Обход BFS внутри того же hostname: по умолчанию до 20 попыток страниц, глубина 2, visited URLs, ограниченная очередь ссылок, пауза между дочерними страницами. Повторный импорт обновляет существующий источник по scoped URL; неизменившиеся дочерние страницы не создают новый индекс. Ручной refresh повторно получает страницу/обход. Страницы, которые не удалось загрузить при обходе, пропускаются; корневой источник показывает import_partial и число успешно полученных страниц. Это счётчик импорта, а не число завершённых embeddings дочерних страниц — у каждой страницы собственный статус. Query-параметры не удаляются, canonical URL из HTML не применяется; удаления исчезнувших страниц и регулярного crawl нет.

## Очередь и версии

knowledge_jobs в PostgreSQL, SELECT FOR UPDATE SKIP LOCKED. Один последовательный worker в процессе, параллельный API. Lease 5 минут, общий deadline задания 3 минуты, fencing UUID. После рестарта просроченный lease возвращается в обработку; прежний token не может публиковать результат. До трёх попыток, временные ошибки — повторы через 10/20 секунд, постоянная валидация — failed. Последняя аварийная попытка переводится в failed после истечения lease. Завершение API отменяет worker и ждёт его выхода до закрытия pool; прерванная lease восстанавливается после срока.

Сервис сохраняет mutable текущую revision источника и immutable готовые chunks активной revision. Промежуточные векторы собираются в памяти в пределах лимитов. В одной транзакции записываются все chunks, меняется active_revision и удаляются прежние chunks. До успешной публикации поиск использует старую активную версию. Устаревшие задания пропускаются, смешивания revisions нет. Удаление источника удаляет chunks/jobs через scoped FK и исключает материал из поиска сразу после commit. Отключение исключает источник немедленно, независимо от готовности нового индекса. Квоты источников/активных заданий на организацию проверяются под advisory lock, в той же транзакции, что enqueue.

## Embeddings и chunking

Провайдер один: Ollama через интерфейс EmbeddingProvider. На реальной BGE-M3 проверена размерность **1024**; SQL хранит vector без заранее угаданной размерности. Каждая запись содержит model, model_version (digest из /api/tags) и dimensions, CHECK vector_dims. Перед и после embedding-запроса проверяется digest; изменённый digest, NaN/Inf, нулевые/разноразмерные векторы и неполный batch не публикуются. Batch по умолчанию 8, embedding HTTP timeout 120 секунд, один запрос одновременно; truncate=false. При смене модели/digest необходимо переиндексировать источники: retrieval фильтруется по текущей модели/digest/размерности, старые модели не смешиваются даже в FTS.

Chunking: примерно 2200 Unicode-символов (ориентир 400–700 токенов), перекрытие 260 символов (~12%), границы абзацев/слов, без LLM. Для PDF chunks не пересекают страницы; сохраняются page и position. Markdown/HTML заголовки остаются текстом в последовательности. Это эвристика размера, не настоящий tokenizer: очень длинные слова/идентификаторы могут увеличить фрагмент, а страницы с коротким текстом дадут маленькие chunks. Не более 1500 chunks на источник.

## Hybrid retrieval

Сначала embedding вопроса. Точный pgvector cosine search по tenant + assistant + enabled + active_revision + model/digest/dimensions, top 30, начальный порог cosine 0.4. FTS: simple Unicode-конфигурация, websearch_to_tsquery, тело + заголовок, top 30. Simple выбран для смешанных языков, чисел и точных терминов; морфологического stemming по языкам нет. RRF с k=60 объединяет списки, возвращает top 5. HNSW/reranker не добавлены. `mode=semantic` возвращает только cosine, `mode=hybrid` (default) — RRF. Score RRF не является вероятностью и не сравним напрямую с cosine score.

Результат: document_id, chunk_id, title, content, URL, PDF page, score/similarity, index_version, model/digest и язык документа. Пустой результат остаётся пустым. Найденный текст — недоверенные данные, его нельзя превращать в системные инструкции при будущем подключении Gemma.

## Настройки

Дополнения к stage 1: EMBEDDING_PROVIDER=ollama, OLLAMA_BASE_URL, EMBEDDING_MODEL=bge-m3, KNOWLEDGE_FILE_ROOT, KNOWLEDGE_MAX_UPLOAD_MB=10, KNOWLEDGE_CHUNK_CHARS=2200, KNOWLEDGE_OVERLAP_CHARS=260, EMBEDDING_BATCH_SIZE=8, IMPORT_MAX_PAGES=20 (верхний предел 20), IMPORT_MAX_DEPTH=2, KNOWLEDGE_MAX_SOURCES=500, KNOWLEDGE_MAX_JOBS=40, RETRIEVAL_MIN_SIMILARITY=0.4. Конфигурация проверяется до запуска API. Разные RU/EU deployments должны иметь собственный Ollama URL, БД и file root; никаких cross-region fallback.

## Воспроизводимая проверка

После создания отдельной site_assistant_test и сборки extractor, из backend:

```powershell
$env:TEST_DATABASE_URL='postgres://site_assistant:local-development-only@localhost:5432/site_assistant_test?sslmode=disable'
$env:PDF_TESTS='1'
$env:PUBLIC_IMPORT_TESTS='1'
$env:REAL_EMBEDDINGS='1'
$env:RAG_REPORT_PATH=(Join-Path (Resolve-Path ../docs) 'rag-evaluation.json')
go test ./... -count=1 -v
go vet ./...
go build ./cmd/api
```

Unit/integration pipeline tests используют явно обозначенный mock; TestRealMultilingualRetrieval — только настоящую Ollama и отдельную схему test DB. [Корпус](../backend/internal/knowledge/testdata/multilingual.json), [фактический отчёт](rag-evaluation.json): 8 документов, 8 вопросов с правильным источником + 1 отсутствующий. Семантический Recall@5=8/8, hybrid Recall@5=8/8; de→ru и en→cs найдены, отсутствующая информация дала 0 результатов в обоих режимах. Неудачных recall-кейсов в этом наборе не было. Порог не подбирался по результату. Маленький корпус даёт оптимистичную оценку; часть вопросов возвращает и нерелевантные дополнительные документы. Перед production нужен расширенный независимый корпус, precision/latency и настройка порога.

Браузер: запустить API с DATABASE_URL=site_assistant_test, frontend, реальную Ollama; `npx playwright test`. Проверяется этап 1 и создание/редактирование статьи, TXT upload, настоящий cross-language search, четыре языка, удаление и размеры 360/768/1440. Без Ollama этот сценарий должен честно не пройти. Повторяя suite, перезапускайте только собственный локальный API или ждите лимита входов 15 минут.

Источники API: [Ollama](https://docs.ollama.com/api/embed), [pgvector](https://github.com/pgvector/pgvector). Poppler — отдельная CLI-утилита GPL-2.0-or-later в Docker, не Go-библиотека; robotstxt — MIT, x/net — BSD. Лицензии утилит следует сохранять при распространении образов.
