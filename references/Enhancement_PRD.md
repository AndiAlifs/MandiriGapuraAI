# Enhancement PRD — GAPURA AI Studio
**Version:** 1.0  
**Date:** April 3, 2026  
**Status:** Proposed  
**Scope:** Post-MVP improvements to close compliance gaps, improve robustness, and expand studio capabilities.

---

## Priority 1 — High Impact (Core Compliance & PRD Gaps)

### ENH-001: Real NER / PII Scrubbing
**Problem:** The current `ScrubPII` function in `internal/pipeline/service.go` uses only regex to detect 16-digit NIK and 10–15 digit account numbers. It does not detect Indonesian names, phone numbers, NPWP (tax ID), addresses, or email addresses. This creates a compliance gap against UU PDP.

**Proposed Solution:**
- Introduce a lightweight NER sidecar service (Python/FastAPI with spaCy `xx_ent_wiki_sm` or a fine-tuned Indonesian NER model).
- The Go pipeline calls the sidecar at `NER_SERVICE_URL` via HTTP before cache check.
- Add environment variable `NER_SERVICE_URL` to config. When empty, fall back to regex-only mode.
- Expand regex patterns to cover: Indonesian phone numbers (`\b08\d{8,11}\b`), NPWP (`\b\d{2}\.\d{3}\.\d{3}\.\d{1}-\d{3}\.\d{3}\b`), and email addresses.

**Affected Files:**
- `internal/pipeline/service.go` — extend `ScrubPII`, add NER HTTP call
- `internal/config/config.go` — add `NERServiceURL`
- New: `ner-service/` Python sidecar (separate directory)

**Compliance Mapping:** UU PDP Article 16 (data minimization before external transmission)

---

### ENH-002: Daily Token Quota Enforcement (Hard 429 Stop)
**Problem:** `Apps_Auth.DailyTokenLimit` exists in the schema, but the pipeline does not check or enforce it. There is no mechanism to block requests once a project exceeds its daily budget.

**Proposed Solution:**
- In `internal/db/repository.go`, add `GetDailyTokenUsage(ctx, projectID, date) (int, error)` that sums `InputTokens + OutputTokens` from `Audit_Logs` where `AppID = ?` and `DATE(Timestamp) = ?`.
- In `pipeline.Service.Process`, after authentication, call this method and compare against `APIKey.DailyTokenLimit`. If exceeded, return `429 Too Many Requests` with body `{"error": "daily token limit exceeded"}`.
- Add `DailyTokenLimit` field to the `db.APIKey` model (already exists in the schema).
- Cache the token usage count in `MemoryCache` with a TTL of 60 seconds to avoid per-request DB queries.

**Affected Files:**
- `internal/db/repository.go` — add `GetDailyTokenUsage`
- `internal/db/model.go` — ensure `DailyTokenLimit` is on `APIKey` struct
- `internal/pipeline/service.go` — add quota gate in `Process`

---

### ENH-003: Streaming Support (SSE / `text/event-stream`)
**Problem:** The pipeline hard-sets `stream: false`, blocking all clients that require streaming (chat UIs, real-time displays).

**Proposed Solution:**
- Detect `"stream": true` in the incoming request before overriding it.
- Add a `ProcessStream` method in `pipeline.Service` that forwards the upstream SSE response to the client using `http.Flusher`.
- Token counting for streaming responses should be done by counting response chunks, with audit logging done asynchronously at end of stream.
- Streaming responses must not be cached.
- Add a separate route handler path in `internal/http/handler.go` that delegates to either `Process` or `ProcessStream` based on the `stream` flag.

**Affected Files:**
- `internal/pipeline/service.go` — add `ProcessStream`
- `internal/http/handler.go` — update `chatCompletions` to branch on stream flag

---

## Priority 2 — Medium Impact (Robustness & Operations)

### ENH-004: Redis / Persistent Cache Backend
**Problem:** `internal/cache/memory.go` is in-process only. Cache is lost on every restart and cannot be shared across multiple gateway instances.

**Proposed Solution:**
- Add a `CacheStore` interface with `Get(key string) ([]byte, bool)` and `Set(key string, value []byte)` methods.
- Implement `RedisCache` backed by `github.com/redis/go-redis/v9`.
- Keep `MemoryCache` as the default when `REDIS_URL` is not set.
- Add `REDIS_URL` and `REDIS_TTL_SECONDS` to config.

**Affected Files:**
- `internal/cache/memory.go` — extract interface
- New: `internal/cache/redis.go`
- `internal/config/config.go` — add `RedisURL`, `RedisTTLSeconds`
- `cmd/gapura/main.go` — choose cache implementation at startup

---

### ENH-005: HTTP Rate Limiting Middleware
**Problem:** No IP-level or per-key request rate limiter exists. A runaway internal script or misconfigured client could saturate the gateway, violating the PRD's DDoS protection requirement.

**Proposed Solution:**
- Implement a token-bucket rate limiter in `internal/http/middleware.go` using `golang.org/x/time/rate`.
- Two tiers:
  - Global IP limiter: configurable via `RATE_LIMIT_GLOBAL_RPS` (default: 100 req/s).
  - Per-API-key limiter: configurable via `RATE_LIMIT_KEY_RPS` (default: 20 req/s). Applied after authentication.
- Return `429 Too Many Requests` with `Retry-After` header when limits are exceeded.

**Affected Files:**
- New: `internal/http/ratelimit.go`
- `internal/http/handler.go` — wire middleware into `Routes()`
- `internal/config/config.go` — add rate limit config fields

---

### ENH-006: Accurate Token Counting (tiktoken-compatible)
**Problem:** Token counts stored in `Audit_Logs` may not match the actual tokens billed by OpenAI because the current implementation estimates tokens rather than using the model's actual tokenizer.

**Proposed Solution:**
- Integrate `github.com/pkoukk/tiktoken-go` for OpenAI models (cl100k_base for `gpt-4o`, `gpt-4o-mini`; p50k_base for older models).
- For Gemini and local Ollama models, use word-count approximation (`len(strings.Fields(text)) * 1.3`) as a fallback.
- Replace estimated token counts in `logUsage` with tokenizer-computed values before writing to `Audit_Logs`.

**Affected Files:**
- New: `internal/tokenizer/tokenizer.go`
- `internal/pipeline/service.go` — replace usage.PromptTokens estimate in `logUsage`
- `go.mod` — add `tiktoken-go` dependency

---

## Priority 3 — Frontend Studio Enhancements

### ENH-007: Prompt Template Management UI
**Problem:** The backend already supports `GetActivePromptTemplate` and prompt template injection in the pipeline, but there is no Angular UI to create, list, activate, or version templates.

**Proposed Solution:**
- New Angular route `/studio/templates` showing a data grid of all prompt templates per project.
- Actions: Create (modal with fields: name, system prompt, temperature, associated project), Set Active, Deactivate, Delete.
- Backend: Add `GET /v1/studio/templates`, `POST /v1/studio/templates`, `PATCH /v1/studio/templates/:id/activate`, `DELETE /v1/studio/templates/:id` endpoints.
- Templates should display version history (readonly) based on `CreatedAt` timestamps.

**Affected Files:**
- New: `frontend-angular/src/app/templates/` component
- `backend-go/internal/http/handler.go` — add template CRUD handlers
- `backend-go/internal/db/repository.go` — add template CRUD methods

---

### ENH-008: Model & Project Admin UI
**Problem:** Adding models to `Model_Registry` or provisioning new `Apps_Auth` entries requires direct SQL or raw API calls. There is no studio UI for these administrative tasks.

**Proposed Solution:**
- New Angular route `/studio/admin` with two tabs:
  - **Models:** List `Model_Registry` rows, add/edit model entries (name, provider, cost rates, local fallback flag).
  - **Projects:** List `Apps_Auth` rows, provision new projects (generates API key on creation), set/reset daily token limits.
- Backend: Add `POST /v1/studio/models`, `PATCH /v1/studio/models/:id` for model management.

**Affected Files:**
- New: `frontend-angular/src/app/admin/` component
- `backend-go/internal/http/handler.go` — add admin handlers
- `backend-go/internal/db/repository.go` — add model upsert methods

---

### ENH-009: Audit Log CSV Export
**Problem:** The audit trail grid is read-only with no export capability. Compliance teams require downloadable records for regulatory review.

**Proposed Solution:**
- Add an `Export CSV` button to the audit trail explorer component.
- Backend: Add `GET /v1/studio/audit-logs/export` that accepts the same filter params as the paginated endpoint but returns `text/csv` with all matching records (no pagination limit).
- Frontend generates a download by creating a blob URL from the response.

**Affected Files:**
- `frontend-angular/src/app/app.component.ts` — add export action
- `backend-go/internal/http/handler.go` — add export handler
- `backend-go/internal/db/repository.go` — add unbounded audit log query

---

## Priority 4 — Strategic / Phase 2

### ENH-010: Gemini as First-Class Provider
**Problem:** `GEMINI_API_KEY` is present in config but Gemini is not a routable provider; all non-local requests go to OpenAI.

**Proposed Solution:**
- Add a `GeminiClient` in `internal/openai/` (or a new `internal/gemini/` package) that maps OpenAI-format messages to the Gemini `generateContent` API format and maps the response back to OpenAI format.
- Route requests with model names prefixed `gemini-*` to the Gemini client.
- Gemini models should appear in `Model_Registry` with `Provider = "Gemini"`.

**Affected Files:**
- New: `internal/gemini/client.go`
- `internal/pipeline/service.go` — update `callCloudWithFallback` to branch on provider

---

### ENH-011: Budget Alert Webhooks
**Problem:** Projects have no warning before hitting their hard quota. A project silently hits 100% and all requests fail with 429 until the next day.

**Proposed Solution:**
- When token usage crosses 80% of `DailyTokenLimit`, fire an async webhook POST to a configurable `Apps_Auth.AlertWebhookURL` (new column) with payload: `{ "project": "...", "usage_pct": 83, "remaining_tokens": 1700 }`.
- Webhooks are sent at most once per day per project (track in memory or a dedicated DB table).
- Add `AlertWebhookURL` to `Apps_Auth` schema and admin UI.

**Affected Files:**
- `internal/db/model.go` — add `AlertWebhookURL` to `APIKey`
- `internal/pipeline/service.go` — add async webhook dispatch after quota check
- `seed.sql` — add `AlertWebhookURL` column migration

---

### ENH-012: Time-Series Usage & Cost Charts
**Problem:** Executive scorecards show only aggregate totals. There is no time-series view of token spend, request volume, or cost per project over time.

**Proposed Solution:**
- Add `GET /v1/studio/analytics/daily-usage?project_id=&days=30` returning daily aggregates of `InputTokens + OutputTokens` and `CalculatedCost` grouped by date and project.
- Frontend: Add a `ng2-charts` (Chart.js wrapper) line chart to the scorecards page showing the last 30 days of spend by project.

**Affected Files:**
- New: `backend-go/internal/db/analytics.go`
- `backend-go/internal/http/handler.go` — add analytics handler
- `frontend-angular/` — add chart component, install `ng2-charts`

---

## Summary Table

| ID | Title | Priority | Effort | Compliance/FinOps Impact |
|----|-------|----------|--------|--------------------------|
| ENH-001 | Real NER / PII Scrubbing | High | Large | UU PDP compliance |
| ENH-002 | Daily Token Quota Enforcement | High | Small | FinOps hard cap |
| ENH-003 | Streaming Support | High | Medium | Developer experience |
| ENH-004 | Redis Persistent Cache | Medium | Small | Reliability, multi-instance |
| ENH-005 | HTTP Rate Limiting Middleware | Medium | Small | DDoS protection |
| ENH-006 | Accurate Token Counting | Medium | Small | Cost accuracy |
| ENH-007 | Prompt Template Management UI | Medium | Medium | Developer experience |
| ENH-008 | Model & Project Admin UI | Medium | Medium | Operability |
| ENH-009 | Audit Log CSV Export | Medium | Small | Compliance reporting |
| ENH-010 | Gemini as First-Class Provider | Low | Medium | Provider diversity |
| ENH-011 | Budget Alert Webhooks | Low | Small | FinOps visibility |
| ENH-012 | Time-Series Usage Charts | Low | Medium | Executive reporting |
