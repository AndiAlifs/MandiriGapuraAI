# Technical Architecture & Reference Guide
**System:** GAPURA AI Studio MVP
**Target Environment:** .NET Core 8.0 (Backend), Angular 18 (Frontend), MySQL 8.0 (Database), Local VPS (8GB VRAM).

## 1. System Request Flow (The GAPURA Pipeline)
*Agent Instruction: Follow this exact sequence when building the `ChatCompletionController`.*

1. **Receive:** Intercept POST request at `/v1/chat/completions`.
2. **Auth & Quota Check:** Validate `Authorization` header against `Apps_Auth` table. Check if today's token usage < `DailyTokenLimit`. Return `429` if exceeded.
3. **Cache Check:** Hash the incoming `messages` array (SHA-256). Check Redis/Memory cache for identical hash within 24 hours. If found, return cached response immediately.
4. **Scrubbing (NER):** Pass prompt to the NER service. Mask Indonesian PII (e.g., replace NIK with `[NIK_MASKED]`, Names with `[NAME_MASKED]`).
5. **Tokenize Input:** Calculate input tokens using `Microsoft.ML.Tokenizers` or `tiktoken` equivalent.
6. **Route & Execute:** - Forward scrubbed prompt to requested Cloud API (e.g., OpenAI).
   - **Fallback Rule:** If Cloud returns 429, 500, or times out (>10s), automatically redirect request to the Local VPS Model (e.g., `http://localhost:11434/api/chat` for Ollama).
7. **Tokenize Output:** Calculate output tokens from the AI response.
8. **Calculate Cost:** Look up `CostPer1kInput` and `CostPer1kOutput` in `Model_Registry`. Compute: `(InputTokens / 1000 * InputRate) + (OutputTokens / 1000 * OutputRate)`.
9. **Audit Logging:** Async write to `Audit_Logs` table (Original Prompt, Scrubbed Prompt, Tokens, Cost, Latency).
10. **Respond:** Return standard OpenAI-formatted JSON to the client.

## 2. Security & Wrapper Architecture Rules
*Agent Instruction: When building the external integration services, enforce these security standards.*
* **Wrapper Implementation:** The .NET Gateway must utilize a secure wrapper architecture to handle incoming payloads, similar to standard internal integrations. 
* **Protocol Support:** Ensure the networking layer is configured to securely handle OpenSSL. (Note: While PSFTP and PSMPT are used in other internal apps, standard HTTPS/TLS 1.2+ is required here for the REST API).
* **Compliance:** The system must strictly adhere to UU ITE and UU PDP regulations by guaranteeing no unmasked PII is transmitted in Step 6 to external cloud providers.

## 3. Database Schema Definitions (MySQL)
*Agent Instruction: Use these exact types when generating Entity Framework Core DbContext and Models.*

**Table: Apps_Auth**
- `AppID` (INT, Primary Key, Auto-Increment)
- `ProjectName` (VARCHAR 100) - e.g., "PopCorn RAG", "ATLAS"
- `Username` (VARCHAR 50, Unique)
- `PasswordHash` (VARCHAR 255)
- `DailyTokenLimit` (INT) - Hard quota per day.

**Table: Model_Registry**
- `ModelID` (INT, Primary Key, Auto-Increment)
- `ModelName` (VARCHAR 100) - e.g., "gpt-4o-mini", "llama3-8b-local"
- `Provider` (VARCHAR 50) - e.g., "OpenAI", "Ollama"
- `CostPer1kInput` (DECIMAL 10,6) - USD cost per 1000 input tokens.
- `CostPer1kOutput` (DECIMAL 10,6) - USD cost per 1000 output tokens.
- `IsLocalFallback` (BOOLEAN) - True if this model runs on the local VPS.

**Table: Audit_Logs**
- `LogID` (BIGINT, Primary Key, Auto-Increment)
- `AppID` (INT, Foreign Key references Apps_Auth)
- `ModelUsed` (VARCHAR 100)
- `OriginalPrompt` (TEXT) - Encrypted at rest if possible.
- `ScrubbedPrompt` (TEXT)
- `ResponseText` (TEXT)
- `InputTokens` (INT)
- `OutputTokens` (INT)
- `CalculatedCost` (DECIMAL 10,6)
- `LatencyMS` (INT)
- `Timestamp` (DATETIME, Default Current_Timestamp)

## 4. API Contract (OpenAI Compatibility)
*Agent Instruction: The gateway MUST accept and return the exact same JSON structure as OpenAI to ensure zero friction for internal apps.*

**Expected POST Request payload:**
```json
{
  "model": "gpt-4o-mini",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello, my NIK is 3171234567890123. Can you help me?"}
  ],
  "temperature": 0.7
}
```

**Expected POST Response payload:**
```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1677652288,
  "model": "gpt-4o-mini",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "Hello! I have masked your NIK for security. How can I help you today?"
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 18,
    "total_tokens": 43
  }
}
```

## 5. Angular 18 UI Structure
```markdown
*Agent Instruction: When generating the Angular 18 UI, structure the components as follows:*

* **Core API Service** (`src/app/core/services/api.service.ts`): Handles communication with the .NET backend.
* **Dashboard** (`src/app/features/dashboard/`): Executive Scorecards (PII scrubbed, Cost Saved).
* **Playground** (`src/app/features/playground/`): Sandbox UI for prompt testing and "Get Code" snippet generation.
* **Audit Trail** (`src/app/features/audit-trail/`): Interactive data grid for `Audit_Logs` with row-expansion for prompt comparison.
```
