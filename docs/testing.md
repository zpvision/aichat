# Результаты проверок этапа 1

Локально, Windows, 12 ноября 2026. Go обновлён с 1.26.5 до 1.27.4, Node 24.19.0, PostgreSQL 17 в Docker. Production и чужие БД не использовались.

- `go test ./... -count=1 -v` с TEST_DATABASE_URL: PASS. Конфигурация, Argon2id, соль и неверный пароль, IDNA/ошибочные домены; интеграционный сценарий с новым случайным schema в site_assistant_test.
- Интеграционный тест: две регистрации и входы, одинаковый ответ повторной регистрации без выдачи сессии, неверный пароль и неизвестная почта, 401 без сессии, создание компаний, проверки owner/admin/operator, чужие tenant IDs для чтения/изменения, невозможность межклиентского FK в БД, draft и запрет active, добавление/нормализация/удаление сайта, все четыре языка профиля, валидация, logout и истёкшая сессия, CSRF, rate limit.
- Миграции применены дважды в изолированной тестовой схеме и дважды через документированный PowerShell helper в новой локальной site_assistant: PASS.
- `go vet ./...`, `go build ./cmd/api`: PASS.
- `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`: после обновления Go до 1.27.4 — No vulnerabilities found. На исходном установленном Go 1.26.5 сканер находил уязвимости стандартной библиотеки, поэтому backend/go.mod теперь требует исправленный toolchain.
- `npm test`: 6 PASS — полнота ключей четырёх словарей, i18next, форматы локалей, API/сетевые ошибки и CSRF-заголовок.
- `npm run build`: TypeScript + Vite PASS. `npm audit`: 0 уязвимостей на момент проверки.
- `npx playwright test`: 2 PASS — регистрация, вход, создание компании/ассистента/сайта, смена RU/CS/DE/EN, отмена/подтверждение удаления сайта, профиль, logout, повторный login; отдельный тест проверяет несовпадение паролей и отображение ошибки API на мобильном экране. На 360/768/1440 px проверено отсутствие горизонтального переполнения документа, сделаны снимки; 360/1440 дополнительно просмотрены визуально. Обнаруженное сжатие карточки сайта на телефоне исправлено и перепроверено.
- `/healthz` и `/readyz`, доступность PostgreSQL проверены локально. Снимки не включены в Git; генерируются повторным браузерным тестом.

Не выполнялись production deployment, нагрузочные испытания, профессиональный аудит безопасности/доступности и реальные почтовые/OIDC/AI/CRM сценарии. Тестовый runner без TEST_DATABASE_URL пропускает integration test: это не эквивалент полной проверке.

## Этап 2

Проверки knowledge выполняются на отдельной site_assistant_test со случайными схемами. Mock-провайдер используется только для воспроизводимых pipeline/queue/isolation tests, никогда не подменяет inference в приложении.

- Unit/integration: UTF-8 TXT/MD, DOCX paragraphs/tables, повреждённые/пустые/подменённые файлы, duplicate upload, chunking/pages, невалидные embeddings, scope/роли, включение/выключение, atomic reindex и сохранение предыдущего индекса при ошибке, model-version filtering, delete cascade, quota, SKIP LOCKED, lease recovery/fencing/retry limit. HTTP auth/isolation tests этапа 1 дополнены knowledge CRUD/permissions.
- PDF_TESTS=1: настоящий ограниченный Poppler, text PDF, многостраничность с page metadata, пустой PDF → ocr_required, повреждённый PDF. PUBLIC_IMPORT_TESTS=1: реальный публичный https://example.com. SSRF/private DNS проверяется production transport; редирект на private адрес, robots, crawl cap/depth/repeat — контролируемым HTTP transport в тесте, без ослабления production сетевых проверок.
- REAL_EMBEDDINGS=1: настоящий Ollama BGE-M3, 1024 dimensions, digest в docs/rag-evaluation.json. Семантический и hybrid Recall@5 **8/8**, включая de→ru/en→cs, точный тариф и стоимость. Для отсутствующей информации — 0 результатов, для другой компании — 0. Это маленький набор, не production benchmark; дополнительные нерелевантные hits встречаются. Результаты отдельно от mock.
- Playwright: **3 сценария** — два прежних + knowledge CRUD, TXT upload, настоящая индексация/поиск, языки RU/EN/CS/DE, deletion, screenshots и отсутствие document overflow 360/768/1440. Снимки knowledge 360/1440 просмотрены визуально.

Итог: `go test ./... -count=1 -v` с TEST_DATABASE_URL, PDF_TESTS, PUBLIC_IMPORT_TESTS, REAL_EMBEDDINGS — PASS; после уточнения паузы/предупреждения частичного crawl его тесты также повторены — PASS. Проверена миграция из схемы 001 с существующим пользователем: hash/locale сохранены, 002 повторно применяется безопасно. `go vet ./...`, `go build ./cmd/api`, `npm test` (6 тестов), TypeScript/Vite production build — PASS. `npm audit` — 0 уязвимостей, `govulncheck` — No vulnerabilities found. Последний browser suite включает также настоящий публичный web import через UI — 3/3 PASS.

Команды и env — docs/rag.md. Для повторного запуска полного browser suite нужен свежий локальный API-процесс либо ожидание login rate limit. Без настоящей Ollama knowledge browser test не считается пройденным. Production, OCR, Gemma generation, большие corpus/нагрузка и полный pentest не проверялись.

## Этап 3 — разделение доказательств

1. Backend: PostgreSQL migration 003/repeat, settings и роли, создание/продолжение/удаление, две последовательные пары, история и порядок, idempotency body/replay, concurrent same-conversation rejection, cancellation/failure/timeout/no partial persistence, cross-company/assistant scope, crash recovery, fallback и Unicode context budgets. HTTP routes проверяют cookie/CSRF, JSON и SSE.
2. Настоящий RAG: при REAL_EMBEDDINGS=1 BGE-M3 извлекает scoped русскую статью для немецкого вопроса, передаёт revision 1, после reindex — revision 2 в **Mock AI request**. В корпусе есть похожая статья другого ассистента, в контекст она не попадает. Предыдущий knowledge multilingual corpus повторён: semantic/hybrid Recall@5=8/8, absent query=0, без изменения thresholds. Это не оценка ответов Gemma.
3. Контракт Ollama: только httptest HTTP server, /api/chat payload, options/think/schema, stream=true/false, NDJSON, thinking discarded, usage, malformed/truncated/error/tool-call response, timeout. Никакого настоящего /api/chat вызова Gemma.
4. Frontend: 13 tests — четыре словаря, API errors, SSE chunked UTF-8/CRLF/completion/error, React demo disclosure/settings/Enter/cancel. TypeScript/production build. Browser suite расширен до 4 сценариев: прежние SaaS/RAG + preview с real BGE-M3 и mock generation, инструкции, два вопроса, independent UI language, reload/history, streaming/cancel/delete и 360/768/1440. Скриншоты проверяются визуально; исправлена классификация отмены во время embedding request (ai_canceled, не embedding_unavailable).
5. Gemma: не проверены качество ответов/языков/grounding/injection, фактические options/think/format/context и GPU latency/load. Старый GPU не использован, моделей не скачивали. Public widget/CRM/leads не реализованы.

Команды — docs/ai-engine.md; TEST_DATABASE_URL только существующая *_test БД. Unit/contract fixtures и MockProvider не являются результатами настоящего inference. Полная регрессия stages 1–2 выполняется с PDF_TESTS=1, PUBLIC_IMPORT_TESTS=1, REAL_EMBEDDINGS=1.

Итоговые проверки этапа 3: полный `go test ./... -count=1 -v` с настоящей BGE-M3 и PDF/public-import флагами — PASS; `go vet`, backend build, TypeScript/Vite build — PASS. `npm test`: 13/13. `npm audit`: 0 уязвимостей, govulncheck: No vulnerabilities found. Playwright: 4/4, включая реальные incremental SSE deltas до complete, persisted canceled state и регрессию этапов 1–2. Все тесты запускались локально; production/GPU не использованы.
