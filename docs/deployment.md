# Локальное окружение и будущие RU/EU

Compose запускает только PostgreSQL 17, bind 127.0.0.1:5432, именованный volume. Backend/frontend — локальные процессы; Redis, GPU, S3 не нужны. Команды — в README. Compose использует отдельную небольшую подсеть, меняемую DOCKER_SUBNET. Не удалять чужие сети и volumes для решения конфликтов.

Backend env: APP_ENV development/production, APP_REGION ru/eu, APP_BRAND_NAME, DEFAULT_LOCALE ru/en/cs/de, PUBLIC_APP_URL (origin без пути/query), HTTP_ADDR, DATABASE_URL. Проверка при запуске; в production PUBLIC_APP_URL обязан быть HTTPS, cookie Secure. `.env` автоматически не читается Go-приложением: Windows helper импортирует переменные, Unix — shell export. В `.env.example` только локальные демонстрационные реквизиты. Frontend получает публичную конфигурацию с сервера; секретов Vite нет.

RU и EU разворачиваются одинаковым исходным кодом, но с разными доменами, DSN, backups и инфраструктурой. Нет межрегиональных запросов или синхронизации персональных данных. Production в этом этапе не развёрнут.

Перед production: HTTPS reverse proxy на одном origin для статического frontend и /api; доверенная сеть к backend; TLS к БД; секреты вне Git; отдельная роль мигратора, ограниченные DB credentials приложения; backup/restore exercise; CSP на статическом frontend, HSTS на HTTPS proxy; мониторинг и нагрузочные тесты. У reverse proxy обязателен IP rate limit login/register: приложение считает peer IP и намеренно игнорирует потенциально подделанный X-Forwarded-For, поэтому без edge limit все запросы прокси попадут в общий лимит. Текущий лимитер хранится в памяти одного процесса, сбрасывается при перезапуске; для масштабирования понадобится общее хранилище или edge-политика.

Срок сессии 7 дней без sliding refresh; очистка раз в час. Остановка API завершает текущие HTTP-запросы в течение 10 секунд и закрывает пул. Миграции применяются отдельной командой до запуска новой версии. Не запускать тесты против production DSN.
