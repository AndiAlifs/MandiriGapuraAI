# Product Requirements Document (PRD)

**Project Name:** GAPURA AI Studio (Gateway for AI Privacy, Utilization, Routing, & Analytics)  
**Target Organization:** Bank Mandiri  
**Phase:** Minimum Viable Product (MVP)  

## 1. Product Vision & Executive Summary
GAPURA AI Studio is an enterprise-grade AI middleware platform serving as the centralized API gateway and developer studio for internal applications utilizing Large Language Models (LLMs). The platform ensures strict compliance with Indonesian data privacy laws (UU ITE & UU PDP) by scrubbing sensitive PII before data leaves the corporate network. It implements static routing with local failover, exact-match caching, and token-level cost tracking to dramatically reduce API overhead, guarantee uptime, and provide executives with a transparent audit trail of all AI utilization.

## 2. Technical Stack & Deployment (MVP)
* **Backend (API Gateway):** Golang
* **Frontend (Studio UI):** Angular
* **Database:** MySQL
* **Local AI Engine:** Ollama / vLLM (Quantized models optimized for an 8GB VRAM VPS constraint)
* **Deployment:** On-premise / Private VPS

## 3. Core Epics & Features

### Epic 1: Core API Gateway & Routing Logic
* **OpenAPI-Compatible REST API:** Expose endpoints mirroring the standard OpenAI API. Internal applications only need to change their `BaseURL` and Header Auth (Username/Password for MVP) to integrate.
* **Static Routing with Local Fallback:** Applications request a specific model tier (e.g., `gpt-4o-mini`). If the external cloud API times out or returns a rate-limit error, the Gateway automatically reroutes the prompt to the in-house local LLM running on the VPS to ensure zero downtime.
* **Exact-Match Caching:** Hash incoming scrubbed prompts. If an identical prompt was processed recently, return the cached response immediately to bypass model compute costs entirely and drop latency to near-zero.

### Epic 2: Security & Compliance (Data Privacy)
* **NER Data Scrubbing:** Intercept all incoming payloads and pass them through a lightweight Named Entity Recognition (NER) model to mask PII (e.g., NIK, Account Numbers, Customer Names) *before* transmission to external cloud providers.
* **Secure Wrapper Architecture:** Utilize a secure wrapper architecture with OpenSSL to handle incoming payloads and enforce modern security protocols, protecting the VPS from internal DDoS or runaway scripts.
* **Dual-Logging Audit Trail:** Store both the *original raw prompt* (for internal security audits) and the *scrubbed prompt* (for model performance tracking) securely in the MySQL database.

### Epic 3: FinOps & Governance
* **Token Counting & Cost Estimation:** Utilize a Golang tokenization library to count exact input and output tokens. Calculate actual costs against a dynamic "Rate Card" in the database.
* **Hard Quotas (Rate Limiting):** Enforce strict daily token limits or request limits per internal project. If a project hits its cap, the Gateway returns a standard `429 Too Many Requests` error to prevent budget overruns.

### Epic 4: GAPURA AI Studio (Angular Frontend)
* **Model Playground:** A developer sandbox to test prompts, visualize token counts, and simulate costs across different models in real-time.
* **"Get Code" Snippet Generator:** A one-click export inside the Playground that produces copy-pasteable integration snippets (Go, cURL) pre-injected with the user's specific API credentials and the GAPURA endpoint.
* **Executive Scorecards:** Simple, high-impact numerical metric cards displaying:
  * *Total PII Entities Scrubbed* (proving UU PDP compliance).
  * *Total API Cost Saved* (calculating dollars saved via caching and local fallback).
* **Audit Trail Explorer:** An interactive data grid showing logs for every request. Features include filtering by project/model, and expanding rows to view the exact latency, calculated cost, original vs. scrubbed prompt, and the AI's exact response.

## 4. Out of Scope for MVP (Phase 2 Roadmap)
* Dynamic routing based on prompt complexity.
* Automated budget routing (seamlessly shifting to free models upon budget exhaustion).
* Prompt template library and version control.
* Complex visual analytics (adoption heatmaps, time-series charts).

## 5. High-Level Database Schema (MySQL)
* `Apps_Auth`: `AppID`, `ProjectName`, `Username`, `PasswordHash`, `DailyTokenLimit`.
* `Model_Registry`: `ModelID`, `ModelName`, `Provider`, `CostPer1kInput`, `CostPer1kOutput`, `IsLocalFallback`.
* `Audit_Logs`: `LogID`, `AppID`, `ModelUsed`, `OriginalPrompt`, `ScrubbedPrompt`, `ResponseText`, `InputTokens`, `OutputTokens`, `CalculatedCost`, `LatencyMS`, `Timestamp`.