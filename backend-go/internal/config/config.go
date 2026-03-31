package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	ListenAddr  string
	DatabaseDSN string
	LogLevel    string
	LogFormat   string
	// OpenAI
	OpenAIBaseURL  string
	OpenAIAPIKey   string
	OpenAIChatPath string
	// Gemini (OpenAI-compatible endpoint)
	GeminiBaseURL  string
	GeminiAPIKey   string
	GeminiChatPath string
	// Local Ollama fallback
	OllamaChatURL  string
	RequestTimeout time.Duration
	CacheTTL       time.Duration
	AuthRealm      string
	// GIN/CORS
	GinMode    string
	CORSOrigin string
	// TLS
	TLSCertFile string
	TLSKeyFile  string
}

func Load() (Config, error) {
	_ = godotenv.Load() // Ignore error if .env doesn't exist or is missing

	// ── Server port ───────────────────────────────────────────────────
	// Accepts PORT or SERVER_PORT (PORT takes precedence).
	port := envOrDefault("PORT", envOrDefault("SERVER_PORT", "8080"))

	// ── Database DSN assembled from individual DB_* vars ─────────────
	dbHost := envOrDefault("DB_HOST", "127.0.0.1")
	dbPort := envOrDefault("DB_PORT", "3306")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := envOrDefault("DB_NAME", "gapura_ai_studio")

	if dbUser == "" {
		return Config{}, fmt.Errorf("DB_USER is required")
	}
	if dbName == "" {
		return Config{}, fmt.Errorf("DB_NAME is required")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC",
		dbUser, dbPassword, dbHost, dbPort, dbName)

	cfg := Config{
		ListenAddr:     ":" + port,
		DatabaseDSN:    dsn,
		LogLevel:       envOrDefault("LOG_LEVEL", "info"),
		LogFormat:      envOrDefault("LOG_FORMAT", "text"),
		OpenAIBaseURL:  envOrDefault("OPENAI_BASE_URL", "https://api.openai.com"),
		OpenAIAPIKey:   os.Getenv("OPENAI_API_KEY"),
		OpenAIChatPath: envOrDefault("OPENAI_CHAT_PATH", "/v1/chat/completions"),
		GeminiBaseURL:  envOrDefault("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"),
		GeminiAPIKey:   os.Getenv("GEMINI_API_KEY"),
		GeminiChatPath: envOrDefault("GEMINI_CHAT_PATH", "/v1beta/openai/chat/completions"),
		OllamaChatURL:  envOrDefault("OLLAMA_CHAT_URL", "http://localhost:11434/api/chat"),
		AuthRealm:      envOrDefault("AUTH_REALM", "gapura"),
		GinMode:        envOrDefault("GIN_MODE", "debug"),
		CORSOrigin:     envOrDefault("CORS_ORIGIN", "http://localhost:4200"),
		TLSCertFile:    os.Getenv("TLS_CERT_FILE"),
		TLSKeyFile:     os.Getenv("TLS_KEY_FILE"),
	}

	timeoutSeconds, err := intFromEnv("REQUEST_TIMEOUT_SECONDS", 10)
	if err != nil {
		return Config{}, err
	}
	cfg.RequestTimeout = time.Duration(timeoutSeconds) * time.Second

	cacheHours, err := intFromEnv("CACHE_TTL_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	cfg.CacheTTL = time.Duration(cacheHours) * time.Hour

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func intFromEnv(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be > 0", key)
	}
	return parsed, nil
}
