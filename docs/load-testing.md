# Local load testing

Run only a local API connected to a disposable *_test DB with Mock AI, no production URLs/GPU. Set LOAD_TEST_DATABASE=confirmed_test, then from root `node scripts/load-local.mjs`. Default LOAD_URL is http://localhost:8080; remote/HTTPS targets are rejected. Restart your own API before auth-heavy runs; do not weaken auth limits. Results in ignored artifacts/stage9/load.json contain timings and fixture IDs, no tokens.

The harness measures a short mixed 10-client workload and ramps public widget configuration/operator list to 10/25/50/100 concurrent clients. It is not endurance or production acceptance. Latest local run 2026-11-14:

| Clients | Requests/sec | p50 ms | p95 ms | p99 ms | HTTP failures |
|---:|---:|---:|---:|---:|---|
|10|565|15.5|25.5|25.5|0|
|25|513|37.3|56.8|57.3|0|
|50|594|71.2|128.6|129.9|0|
|100|545|167.6|293.5|297.8|25 × 429|

429 responses are genuine protection results, not successful throughput. Mixed samples: sessions/SSE/replay p95 1123ms (11 HTTP429); operator history/take/reply p95 47ms (nine HTTP409 competing takes); external create/replay/feed p95 118ms, no HTTP errors; knowledge hybrid search p95 2114ms, no HTTP errors. Local receiver accepted 11 deliveries with zero invalid HMAC signatures. SSE error events are not separately classified by the HTTP-only latency recorder; functional SSE correctness uses browser/PG regression tests.

Client RSS at end: 84,344,832 bytes. Separate short API sampling captured 109–161MB RSS and cumulative CPU 3.23–4.53 seconds (sampling partially overlaps the mixed run). Post-run PostgreSQL snapshot: 21 connections including diagnostic client, zero active transaction age, zero database deadlocks. Pool limit is20. These are sparse samples, not per-transaction latency percentiles or a leak/endurance conclusion. Original raw aggregate results and samples are in stage9-results.json; fixture credentials/tokens are excluded. Cross-tenant correctness is covered by existing PG tests, not inferred from timings. Do not infer Gemma/RTX4090 capacity from these numbers.

Inspect protected /metrics for PG connections/long transactions, active AI/reservations, knowledge and webhook queues/stale leases. Compare before/during/after an eventual sustained run. Existing PG concurrency tests protect three-of-ten quota admission, handoff and duplicate lead creation; they do not replace a production load test.
