# Independent RU/EU deployment plan

One code version, two independent installations; no shared user/lead/document DB, storage, key, backup or GPU. No deployment is performed by this stage.

```
Browser -> regional TLS reverse proxy -> static cabinet + widget assets
                                    -> Go API (one pilot instance)
Go -> regional PostgreSQL/pgvector + private original-file volume
Go -> regional BGE-M3 endpoint
Go -> regional Gemma endpoint (disabled until Stage 9B)
```

For each region separately: APP_ENV=production, explicit APP_REGION=ru/eu, APP_BRAND_NAME, DEFAULT_LOCALE independent of region, public HTTPS PUBLIC_APP_URL and WIDGET_PUBLIC_URL (real domains, no test names); DATABASE_URL with strong credentials and sslmode=verify-full/certificate trust; private writable KNOWLEDGE_FILE_ROOT and WIDGET_FILE_ROOT; built WIDGET_DIST_DIR; independent OLLAMA_BASE_URL/AI_BASE_URL, EMBEDDING_MODEL=bge-m3, AI_PROVIDER=disabled until tested. Use existing bounded AI/import/widget settings. Set stable WEBHOOK_ENCRYPTION_KEY only from secret storage; empty is allowed only without encrypted endpoints, leaving webhook setup unavailable. METRICS_TOKEN is optional: empty disables the endpoint, otherwise >=32 random characters and private monitoring route only. RETENTION_SESSION_DAYS defaults 7. Never place raw BILLING_ADMIN_TOKEN in API environment.

Reverse proxy must terminate TLS, set HSTS/CSP for cabinet assets, restrict metrics/admin networks, preserve SSE with buffering disabled and finite write/read timeouts. Apply edge IP login/register and widget limits. Backend intentionally ignores forwarded IP headers; behind a proxy its local limiter otherwise sees one peer. Keep only one API instance during pilot; PG commercial quotas are shared but process-local protective limits are not. No Redis is required for one instance. Before multiple instances, implement shared edge limits or a reviewed PG limiter.

Provision a separate migration role and restricted application role, bounded pool (currently max20), backups and encrypted key escrow. Run migrations explicitly with stopped/compatible writers. Build cabinet/widget, apply migrations, validate configuration, restore-test privately, then enable traffic. No public subscription-grant endpoint exists. Production mock, localhost widget bypass and local webhook receiver are forbidden.

For RU use Russian infrastructure/storage/AI and approved local legal/payment settings; for EU use EU equivalents. Neither region may silently call the other's inference/storage. Configured endpoints are deployment-trusted: network egress/firewall policy must enforce residency. Payments remain unconfigured. Monitoring: /healthz process liveness; /readyz PostgreSQL and writable storage, not Gemma availability; /metrics Bearer token, never via public ingress. Alert on growing queue/stale lease, repeated 5xx, quota rejections and long transactions. Review logs by X-Request-ID without collecting bodies/headers/query strings.
