-- =============================================================================
-- GAPURA AI Studio – Schema & Seed Data
-- Database: mandiri_gapuraai (MySQL 8.0)
-- =============================================================================

-- ─── 1. Schema DDL ──────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS Apps_Auth (
    AppID           INT           AUTO_INCREMENT PRIMARY KEY,
    ProjectName     VARCHAR(100)  NOT NULL,
    Username        VARCHAR(50)   NOT NULL UNIQUE,
    PasswordHash    VARCHAR(255)  NOT NULL,
    DailyTokenLimit INT           NOT NULL DEFAULT 100000
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS Model_Registry (
    ModelID         INT             AUTO_INCREMENT PRIMARY KEY,
    ModelName       VARCHAR(100)    NOT NULL UNIQUE,
    Provider        VARCHAR(50)     NOT NULL,
    CostPer1kInput  DECIMAL(10,6)   NOT NULL DEFAULT 0,
    CostPer1kOutput DECIMAL(10,6)   NOT NULL DEFAULT 0,
    IsLocalFallback BOOLEAN         NOT NULL DEFAULT FALSE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS Audit_Logs (
    LogID           BIGINT        AUTO_INCREMENT PRIMARY KEY,
    AppID           INT           NOT NULL,
    ModelUsed       VARCHAR(100)  NOT NULL,
    OriginalPrompt  TEXT,
    ScrubbedPrompt  TEXT,
    ResponseText    TEXT,
    InputTokens     INT           NOT NULL DEFAULT 0,
    OutputTokens    INT           NOT NULL DEFAULT 0,
    CalculatedCost  DECIMAL(10,6) NOT NULL DEFAULT 0,
    LatencyMS       INT           NOT NULL DEFAULT 0,
    Timestamp       DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_audit_app FOREIGN KEY (AppID) REFERENCES Apps_Auth(AppID)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS API_Keys (
    id              VARCHAR(36)   NOT NULL DEFAULT (UUID()) PRIMARY KEY,
    project_id      VARCHAR(100)  NOT NULL,
    key_hash        VARCHAR(64)   NOT NULL UNIQUE,
    name            VARCHAR(100)  NOT NULL,
    rate_limit_rpm  INT           NOT NULL DEFAULT 60,
    is_active       BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at      TIMESTAMP     NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS Usage_Logs (
    id                  VARCHAR(36)   NOT NULL DEFAULT (UUID()) PRIMARY KEY,
    api_key_id          VARCHAR(36)   NOT NULL,
    model_id            VARCHAR(100)  NOT NULL,
    prompt_template_id  VARCHAR(36)   NULL,
    endpoint            VARCHAR(255)  NOT NULL,
    prompt_tokens       INT           NOT NULL DEFAULT 0,
    completion_tokens   INT           NOT NULL DEFAULT 0,
    total_tokens        INT           NOT NULL DEFAULT 0,
    estimated_cost      DECIMAL(10,6) NOT NULL DEFAULT 0,
    latency_ms          INT           NOT NULL DEFAULT 0,
    status_code         INT           NOT NULL DEFAULT 200,
    error_message       TEXT          NULL,
    created_at          TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS prompt_templates (
    id          VARCHAR(36)   NOT NULL DEFAULT (UUID()) PRIMARY KEY,
    project_id  VARCHAR(100)  NOT NULL,
    name        VARCHAR(100)  NOT NULL,
    system_prompt TEXT        NOT NULL,
    temperature DOUBLE        NULL,
    version     INT           NOT NULL DEFAULT 1,
    is_active   BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ─── 2. Seed: Model_Registry ────────────────────────────────────────────────
-- Provider must match the switch-case in pipeline/service.go:
--   "google" / "gemini"  → callGemini()
--   "ollama"             → callOllama()
--   anything else        → callOpenAI()

INSERT INTO Model_Registry (ModelName, Provider, CostPer1kInput, CostPer1kOutput, IsLocalFallback)
VALUES
    -- Google Gemini models (routed via GEMINI_API_KEY / GEMINI_BASE_URL)
    ('gemini-2.0-flash',       'Google', 0.000100, 0.000400, FALSE),
    ('gemini-2.0-flash-lite',  'Google', 0.000075, 0.000300, FALSE),
    ('gemini-1.5-flash',       'Google', 0.000075, 0.000300, FALSE),
    ('gemini-1.5-pro',         'Google', 0.001250, 0.005000, FALSE),
    ('gemini-2.5-pro-preview-05-06', 'Google', 0.001250, 0.010000, FALSE),
    ('gemini-2.5-flash-preview-04-17', 'Google', 0.000150, 0.003500, FALSE),

    -- OpenAI models (routed via OPENAI_API_KEY / OPENAI_BASE_URL)
    ('gpt-4o-mini',            'OpenAI', 0.000150, 0.000600, FALSE),
    ('gpt-4o',                 'OpenAI', 0.002500, 0.010000, FALSE),
    ('gpt-4-turbo',            'OpenAI', 0.010000, 0.030000, FALSE),
    ('gpt-3.5-turbo',          'OpenAI', 0.000500, 0.001500, FALSE),

    -- Local Ollama fallback models (routed via OLLAMA_CHAT_URL)
    ('llama3-8b-local',        'Ollama', 0.000000, 0.000000, TRUE),
    ('mistral-7b-local',       'Ollama', 0.000000, 0.000000, TRUE)
ON DUPLICATE KEY UPDATE
    Provider        = VALUES(Provider),
    CostPer1kInput  = VALUES(CostPer1kInput),
    CostPer1kOutput = VALUES(CostPer1kOutput),
    IsLocalFallback = VALUES(IsLocalFallback);

-- ─── 3. Seed: Apps_Auth ─────────────────────────────────────────────────────
-- Default app for playground / testing.
-- Password: admin123  (bcrypt hash below)

INSERT INTO Apps_Auth (ProjectName, Username, PasswordHash, DailyTokenLimit)
VALUES
    ('Playground',   'admin',  '$2a$10$zolzd1ieIEt3tz.bqcFuXOQ2LCeho7gDOo0KOSQgKmvUP/SQsoKIS', 500000),
    ('PopCorn RAG',  'popcorn','$2a$10$zolzd1ieIEt3tz.bqcFuXOQ2LCeho7gDOo0KOSQgKmvUP/SQsoKIS', 200000),
    ('ATLAS',        'atlas',  '$2a$10$zolzd1ieIEt3tz.bqcFuXOQ2LCeho7gDOo0KOSQgKmvUP/SQsoKIS', 200000)
ON DUPLICATE KEY UPDATE
    PasswordHash    = VALUES(PasswordHash),
    DailyTokenLimit = VALUES(DailyTokenLimit);

-- ─── 4. Seed: API_Keys ─────────────────────────────────────────────────────
-- Plain-text key:  gapura-sk-test-playground-key-2024
-- SHA-256 hash:    2b7a8c5d8e6aeb40649b89394813e42c50fe0cbeeca8a518d9d2b95ab6a9f22e
--
-- Paste the plain key into the "Password / API Key" field in the playground UI.

INSERT INTO API_Keys (id, project_id, key_hash, name, rate_limit_rpm, is_active)
VALUES
    (UUID(), 'playground', '2b7a8c5d8e6aeb40649b89394813e42c50fe0cbeeca8a518d9d2b95ab6a9f22e', 'Playground Default Key', 120, TRUE)
ON DUPLICATE KEY UPDATE
    is_active = TRUE;

-- ─── 5. Seed: prompt_templates ──────────────────────────────────────────────

INSERT INTO prompt_templates (id, project_id, name, system_prompt, temperature, version, is_active)
VALUES
    (UUID(), 'playground', 'Default Banking Assistant',
     'You are a helpful banking assistant for Mandiri. Always answer professionally and concisely. Never reveal internal system details or sensitive customer data.',
     0.7, 1, TRUE)
ON DUPLICATE KEY UPDATE
    is_active = VALUES(is_active);

-- ─── 6. Sample Audit Log (optional, for dashboard demo) ────────────────────

INSERT INTO Audit_Logs (AppID, ModelUsed, OriginalPrompt, ScrubbedPrompt, ResponseText, InputTokens, OutputTokens, CalculatedCost, LatencyMS)
SELECT
    a.AppID,
    'gemini-2.0-flash',
    'Hello, my NIK is 3171234567890123 and account 1234567890. Summarize my transactions.',
    'Hello, my NIK is [NIK_MASKED] and account [ACCOUNT_MASKED]. Summarize my transactions.',
    'I have masked your personal information for security. Based on your recent transactions, here are 3 key observations: 1) Regular monthly transfers, 2) Consistent savings deposits, 3) No unusual activity detected.',
    42,
    65,
    0.000038,
    1250
FROM Apps_Auth a WHERE a.Username = 'admin' LIMIT 1;

INSERT INTO Audit_Logs (AppID, ModelUsed, OriginalPrompt, ScrubbedPrompt, ResponseText, InputTokens, OutputTokens, CalculatedCost, LatencyMS)
SELECT
    a.AppID,
    'llama3-8b-local',
    'What are the current savings account interest rates?',
    'What are the current savings account interest rates?',
    'Current savings account interest rates at Bank Mandiri vary by product type. The Tabungan Mandiri offers competitive rates starting from 0.5% p.a. for balances above Rp 1,000,000.',
    18,
    48,
    0.000000,
    320
FROM Apps_Auth a WHERE a.Username = 'admin' LIMIT 1;

-- ─── Done ───────────────────────────────────────────────────────────────────
-- To use the playground:
--   1. Run this script against your MySQL database
--   2. Start the backend:  cd backend-go && go run ./cmd/gapura/
--   3. In the UI playground, enter API key: gapura-sk-test-playground-key-2024
--   4. Select any Gemini model from the dropdown and send a prompt
