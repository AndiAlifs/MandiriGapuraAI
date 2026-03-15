package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"gapura/backend-go/internal/cache"
	"gapura/backend-go/internal/config"
	"gapura/backend-go/internal/db"
	"gapura/backend-go/internal/openai"
)

type Service struct {
	cfg        config.Config
	repo       *db.Repository
	cacheStore *cache.MemoryCache
	httpClient *http.Client
}

type ProcessInput struct {
	App         db.AppAuth
	RawBody     []byte
	Request     openai.ChatCompletionRequest
	RequestHash string
}

type ProcessOutput struct {
	StatusCode int
	Body       []byte
	FromCache  bool
}

func NewService(cfg config.Config, repo *db.Repository, cacheStore *cache.MemoryCache) *Service {
	return &Service{
		cfg:        cfg,
		repo:       repo,
		cacheStore: cacheStore,
		httpClient: &http.Client{Timeout: cfg.RequestTimeout},
	}
}

func (s *Service) AuthenticateAndCheckQuota(ctx context.Context, username, password string) (*db.AppAuth, error) {
	app, err := s.repo.AuthenticateApp(ctx, username, password)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, nil
	}

	usage, err := s.repo.DailyTokenUsage(ctx, app.AppID)
	if err != nil {
		return nil, err
	}
	if usage >= app.DailyTokenLimit {
		return &db.AppAuth{
			AppID:           -1,
			ProjectName:     app.ProjectName,
			Username:        app.Username,
			PasswordHash:    app.PasswordHash,
			DailyTokenLimit: app.DailyTokenLimit,
		}, nil
	}

	return app, nil
}

func (s *Service) HashRequest(req openai.ChatCompletionRequest) string {
	payload := struct {
		Model    string           `json:"model"`
		Messages []openai.Message `json:"messages"`
	}{
		Model:    req.Model,
		Messages: req.Messages,
	}
	buf, _ := json.Marshal(payload)
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}

func (s *Service) Process(ctx context.Context, input ProcessInput) (ProcessOutput, error) {
	if cached, ok := s.cacheStore.Get(input.RequestHash); ok {
		return ProcessOutput{StatusCode: http.StatusOK, Body: cached, FromCache: true}, nil
	}

	start := time.Now()
	body, status, modelUsed, usage, err := s.callCloudWithFallback(ctx, input.RawBody, input.Request)
	if err != nil {
		return ProcessOutput{}, err
	}

	if status >= 200 && status < 300 {
		s.cacheStore.Set(input.RequestHash, body)
	}

	go s.logAudit(input, body, modelUsed, usage, int(time.Since(start).Milliseconds()))

	return ProcessOutput{StatusCode: status, Body: body, FromCache: false}, nil
}

func (s *Service) callCloudWithFallback(ctx context.Context, rawBody []byte, req openai.ChatCompletionRequest) ([]byte, int, string, openai.UsageDetails, error) {
	provider, err := s.repo.ModelProvider(ctx, req.Model)
	if err != nil {
		log.Printf("model registry lookup failed, defaulting to OpenAI: %v", err)
		provider = "OpenAI"
	}

	var cloudBody []byte
	var cloudStatus int
	var cloudUsage openai.UsageDetails
	var cloudErr error

	switch strings.ToLower(provider) {
	case "gemini":
		cloudBody, cloudStatus, cloudUsage, cloudErr = s.callGemini(ctx, rawBody)
	default:
		cloudBody, cloudStatus, cloudUsage, cloudErr = s.callOpenAI(ctx, rawBody)
	}

	if cloudErr == nil && cloudStatus >= 200 && cloudStatus < 300 {
		return cloudBody, cloudStatus, req.Model, cloudUsage, nil
	}

	if cloudErr != nil {
		log.Printf("cloud call error, switching to local fallback: %v", cloudErr)
	} else if cloudStatus != http.StatusTooManyRequests && cloudStatus < 500 {
		return cloudBody, cloudStatus, req.Model, cloudUsage, nil
	}

	fallbackModel, err := s.repo.LocalFallbackModel(ctx)
	if err != nil {
		return nil, 0, "", openai.UsageDetails{}, err
	}

	fallbackBody, fallbackUsage, err := s.callOllama(ctx, req, fallbackModel)
	if err != nil {
		if cloudErr != nil {
			return nil, 0, "", openai.UsageDetails{}, fmt.Errorf("cloud and fallback both failed: cloud=%w fallback=%v", cloudErr, err)
		}
		return nil, 0, "", openai.UsageDetails{}, err
	}

	return fallbackBody, http.StatusOK, fallbackModel, fallbackUsage, nil
}

// callProvider is the shared HTTP call used by callOpenAI and callGemini.
func (s *Service) callProvider(ctx context.Context, rawBody []byte, endpointURL, authHeader, authValue string) ([]byte, int, openai.UsageDetails, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(rawBody))
	if err != nil {
		return nil, 0, openai.UsageDetails{}, err
	}
	req.Header.Set(authHeader, authValue)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, openai.UsageDetails{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, openai.UsageDetails{}, err
	}

	parsed, err := openai.DecodeResponse(body)
	if err != nil {
		return body, resp.StatusCode, openai.UsageDetails{}, nil
	}
	return body, resp.StatusCode, parsed.Usage, nil
}

func (s *Service) callOpenAI(ctx context.Context, rawBody []byte) ([]byte, int, openai.UsageDetails, error) {
	if s.cfg.OpenAIAPIKey == "" {
		return nil, 0, openai.UsageDetails{}, fmt.Errorf("OPENAI_API_KEY not configured")
	}
	endpoint := strings.TrimRight(s.cfg.OpenAIBaseURL, "/") + s.cfg.OpenAIChatPath
	return s.callProvider(ctx, rawBody, endpoint, "Authorization", "Bearer "+s.cfg.OpenAIAPIKey)
}

func (s *Service) callGemini(ctx context.Context, rawBody []byte) ([]byte, int, openai.UsageDetails, error) {
	if s.cfg.GeminiAPIKey == "" {
		return nil, 0, openai.UsageDetails{}, fmt.Errorf("GEMINI_API_KEY not configured")
	}
	// Gemini's OpenAI-compatible endpoint uses standard Bearer auth.
	endpoint := strings.TrimRight(s.cfg.GeminiBaseURL, "/") + s.cfg.GeminiChatPath
	return s.callProvider(ctx, rawBody, endpoint, "Authorization", "Bearer "+s.cfg.GeminiAPIKey)
}

func (s *Service) callOllama(ctx context.Context, req openai.ChatCompletionRequest, localModel string) ([]byte, openai.UsageDetails, error) {
	payload := openai.OllamaChatRequest{
		Model:    localModel,
		Messages: req.Messages,
		Stream:   false,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, openai.UsageDetails{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.OllamaChatURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, openai.UsageDetails{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, openai.UsageDetails{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, openai.UsageDetails{}, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, openai.UsageDetails{}, err
	}

	var ollamaResp openai.OllamaChatResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return nil, openai.UsageDetails{}, err
	}

	promptTokens := ollamaResp.PromptEvalCount
	if promptTokens == 0 {
		promptTokens = estimatePromptTokens(req)
	}
	completionTokens := ollamaResp.EvalCount
	if completionTokens == 0 {
		completionTokens = maxInt(1, len(ollamaResp.Message.Content)/4)
	}

	compat := openai.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-fallback-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   localModel,
		Choices: []openai.Choice{
			{
				Index: 0,
				Message: openai.Message{
					Role:    "assistant",
					Content: ollamaResp.Message.Content,
				},
				FinishReason: "stop",
			},
		},
		Usage: openai.UsageDetails{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}

	encodedCompat, err := json.Marshal(compat)
	if err != nil {
		return nil, openai.UsageDetails{}, err
	}
	return encodedCompat, compat.Usage, nil
}

func (s *Service) logAudit(input ProcessInput, responseBody []byte, modelUsed string, usage openai.UsageDetails, latencyMS int) {
	responseText := extractResponseText(responseBody)
	prompt := flattenPrompt(input.Request)

	err := s.repo.InsertAuditLog(context.Background(), db.AuditLogInput{
		AppID:          input.App.AppID,
		ModelUsed:      modelUsed,
		OriginalPrompt: prompt,
		ScrubbedPrompt: prompt,
		ResponseText:   responseText,
		InputTokens:    usage.PromptTokens,
		OutputTokens:   usage.CompletionTokens,
		CalculatedCost: 0,
		LatencyMS:      latencyMS,
	})
	if err != nil {
		log.Printf("audit log insert failed: %v", err)
	}
}

func flattenPrompt(req openai.ChatCompletionRequest) string {
	parts := make([]string, 0, len(req.Messages))
	for _, msg := range req.Messages {
		parts = append(parts, msg.Role+": "+msg.Content)
	}
	return strings.Join(parts, "\n")
}

func extractResponseText(raw []byte) string {
	resp, err := openai.DecodeResponse(raw)
	if err != nil || len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
}

func estimatePromptTokens(req openai.ChatCompletionRequest) int {
	totalChars := 0
	for _, m := range req.Messages {
		totalChars += len(m.Content)
	}
	return maxInt(1, totalChars/4)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
