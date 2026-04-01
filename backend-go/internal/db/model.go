package db

import (
	"time"
)

type Project struct {
	ID          string    `db:"id" json:"id"`
	Name        string    `db:"name" json:"name"`
	Description string    `db:"description" json:"description"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time `db:"updated_at" json:"updatedAt"`
}

type AIModel struct {
	ID                  string    `db:"id" json:"id"`
	Provider            string    `db:"provider" json:"provider"`
	ModelName           string    `db:"model_name" json:"modelName"`
	PromptCostPer1K     float64   `db:"prompt_cost_per_1k" json:"promptCostPer1K"`
	CompletionCostPer1K float64   `db:"completion_cost_per_1k" json:"completionCostPer1K"`
	IsActive            bool      `db:"is_active" json:"isActive"`
	CreatedAt           time.Time `db:"created_at" json:"createdAt"`
}

type APIKey struct {
	ID           string     `db:"id" json:"id"`
	ProjectID    string     `db:"project_id" json:"projectId"`
	KeyHash      string     `db:"key_hash" json:"-"` // Hidden from JSON
	Name         string     `db:"name" json:"name"`
	RateLimitRPM int        `db:"rate_limit_rpm" json:"rateLimitRpm"`
	IsActive     bool       `db:"is_active" json:"isActive"`
	CreatedAt    time.Time  `db:"created_at" json:"createdAt"`
	ExpiresAt    *time.Time `db:"expires_at" json:"expiresAt,omitempty"`
}

type UsageLog struct {
	ID               string    `db:"id" json:"id"`
	APIKeyID         string    `db:"api_key_id" json:"apiKeyId"`
	ModelID          string    `db:"model_id" json:"modelId"`
	PromptTemplateID *string   `db:"prompt_template_id" json:"promptTemplateId,omitempty"`
	Endpoint         string    `db:"endpoint" json:"endpoint"`
	PromptTokens     int       `db:"prompt_tokens" json:"promptTokens"`
	CompletionTokens int       `db:"completion_tokens" json:"completionTokens"`
	TotalTokens      int       `db:"total_tokens" json:"totalTokens"`
	EstimatedCost    float64   `db:"estimated_cost" json:"estimatedCost"`
	LatencyMs        int       `db:"latency_ms" json:"latencyMs"`
	StatusCode       int       `db:"status_code" json:"statusCode"`
	ErrorMessage     *string   `db:"error_message" json:"errorMessage,omitempty"`
	CreatedAt        time.Time `db:"created_at" json:"createdAt"`
}

type PromptTemplate struct {
	ID           string   `db:"id" json:"id"`
	ProjectID    string   `db:"project_id" json:"projectId"`
	Name         string   `db:"name" json:"name"`
	SystemPrompt string   `db:"system_prompt" json:"systemPrompt"`
	Temperature  *float64 `db:"temperature" json:"temperature,omitempty"`
	Version      int      `db:"version" json:"version"`
}
