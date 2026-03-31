package config

import "testing"

func TestLoad_AllowsLocalOnlyMode(t *testing.T) {
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "3306")
	t.Setenv("DB_USER", "tester")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "gapura_ai_studio")
	t.Setenv("PORT", "8080")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("REQUEST_TIMEOUT_SECONDS", "10")
	t.Setenv("CACHE_TTL_HOURS", "24")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config load to succeed in local-only mode, got error: %v", err)
	}

	if cfg.OpenAIAPIKey != "" || cfg.GeminiAPIKey != "" {
		t.Fatalf("expected cloud API keys to be empty in test setup")
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("expected listen addr :8080, got %q", cfg.ListenAddr)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected default log level info, got %q", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Fatalf("expected default log format text, got %q", cfg.LogFormat)
	}
}
