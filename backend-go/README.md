# GAPURA Epic 1 Go Backend

This service implements Epic 1 from the PRD:
- OpenAI-compatible endpoint: POST /v1/chat/completions
- Static routing to OpenAI with automatic local fallback to Ollama
- Exact-match response caching based on request hash
- Header authentication with username/password (Basic auth or X-Username/X-Password)
- Daily token quota check using Apps_Auth and Audit_Logs

## 1) Environment Variables

- GAPURA_ADDR=:5000
- GAPURA_DB_DSN=root:password@tcp(localhost:3306)/gapura_ai_studio?parseTime=true&charset=utf8mb4&loc=UTC
- OPENAI_API_KEY=your_openai_api_key
- OPENAI_BASE_URL=https://api.openai.com
- OPENAI_CHAT_PATH=/v1/chat/completions
- OLLAMA_CHAT_URL=http://localhost:11434/api/chat
- REQUEST_TIMEOUT_SECONDS=10
- CACHE_TTL_HOURS=24

## 2) Run

go mod tidy
go run ./cmd/gapura

## 3) Test Endpoint

Use Basic auth (base64 of username:password) and OpenAI-compatible payload.

POST http://localhost:5000/v1/chat/completions
