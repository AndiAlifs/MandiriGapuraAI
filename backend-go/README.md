# GAPURA Epic 1 Go Backend

This service implements Epic 1 from the PRD:
- OpenAI-compatible endpoint: POST /v1/chat/completions
- Auth provisioning endpoint: POST /v1/studio/apps-auth
- Provider routing from Model_Registry (OpenAI or Gemini) with automatic local fallback to Ollama
- Exact-match response caching based on request hash
- Header authentication with username/password (Basic auth or X-Username/X-Password)
- Daily token quota check using Apps_Auth and Audit_Logs
- Configurable CORS middleware (including browser preflight)

## 1) Environment Variables

Server
- PORT or SERVER_PORT (default: 8080)
- AUTH_REALM (default: gapura)
- CORS_ORIGIN (single origin, comma-separated origins, or *)

Database
- DB_HOST (default: 127.0.0.1)
- DB_PORT (default: 3306)
- DB_USER (required)
- DB_PASSWORD
- DB_NAME (default: gapura_ai_studio)

Providers
- OPENAI_API_KEY (optional)
- OPENAI_BASE_URL (default: https://api.openai.com)
- OPENAI_CHAT_PATH (default: /v1/chat/completions)
- GEMINI_API_KEY (optional)
- GEMINI_BASE_URL (default: https://generativelanguage.googleapis.com)
- GEMINI_CHAT_PATH (default: /v1beta/openai/chat/completions)
- OLLAMA_CHAT_URL (default: http://localhost:11434/api/chat)

Runtime
- REQUEST_TIMEOUT_SECONDS (default: 10)
- CACHE_TTL_HOURS (default: 24)

Notes
- Local-only mode is supported by leaving both OPENAI_API_KEY and GEMINI_API_KEY empty.

## 2) Run

go mod tidy
go run ./cmd/gapura

## 3) Test Endpoint

Use Basic auth (base64 of username:password) and OpenAI-compatible payload.

POST http://localhost:5000/v1/chat/completions

## 4) Create Auth Credential

Create an Apps_Auth record for playground login.

POST http://localhost:8080/v1/studio/apps-auth

```json
{
	"projectName": "PopCorn RAG",
	"username": "playground-user",
	"password": "playground-pass",
	"dailyTokenLimit": 500000
}
```

Notes
- Password is stored as bcrypt hash.
- Duplicate usernames return HTTP 409.
