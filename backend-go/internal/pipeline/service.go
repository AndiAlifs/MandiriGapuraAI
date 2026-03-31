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
	"regexp"
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
	App       db.AppAuth
	RawBody   []byte
	Request   openai.ChatCompletionRequest
	RequestID string
}

var (
	nikPattern     = regexp.MustCompile(`\b\d{16}\b`)
	accountPattern = regexp.MustCompile(`\b\d{10,15}\b`)
)

func ScrubPII(text string) (string, int) {
	maskedCount := 0
	scrubbed := nikPattern.ReplaceAllStringFunc(text, func(s string) string {
		maskedCount++
		return "[NIK_MASKED]"
	})
	scrubbed = accountPattern.ReplaceAllStringFunc(scrubbed, func(s string) string {
		maskedCount++
		return "[ACCOUNT_MASKED]"
	})
	return scrubbed, maskedCount
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

func (s *Service) CreateAppAuth(ctx context.Context, in db.CreateAppAuthInput) (*db.AppAuth, error) {
	return s.repo.CreateAppAuth(ctx, in)
}

func (s *Service) StudioScorecards(ctx context.Context) (db.StudioScorecards, error) {
	return s.repo.GetStudioScorecards(ctx)
}

func (s *Service) StudioAuditLogs(ctx context.Context, filter db.AuditLogFilter) ([]db.AuditLogRecord, error) {
	return s.repo.ListAuditLogs(ctx, filter)
}

func (s *Service) StudioModels(ctx context.Context) ([]db.ModelInfo, error) {
	return s.repo.ListModelRegistry(ctx)
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
	rid := input.RequestID

	// 1. Capture original prompt before scrubbing
	originalPrompt := flattenPrompt(input.Request)

	// 2. Apply NER scrubbing to all messages
	var totalMasked int
	for i, msg := range input.Request.Messages {
		scrubbedContent, masked := ScrubPII(msg.Content)
		if masked > 0 {
			input.Request.Messages[i].Content = scrubbedContent
			totalMasked += masked
		}
	}

	if totalMasked > 0 {
		log.Printf("[%s] pipeline: PII scrubbing complete — %d entities masked", rid, totalMasked)

		// Re-marshal the modified raw body to preserve unknown fields
		var raw map[string]interface{}
		if err := json.Unmarshal(input.RawBody, &raw); err == nil {
			if msgs, ok := raw["messages"].([]interface{}); ok {
				for i, m := range msgs {
					if mmap, ok := m.(map[string]interface{}); ok {
						if _, hasContent := mmap["content"]; hasContent {
							mmap["content"] = input.Request.Messages[i].Content
						}
					}
				}
			}
			if newBody, err := json.Marshal(raw); err == nil {
				input.RawBody = newBody
			}
		}
	}

	scrubbedPrompt := flattenPrompt(input.Request)
	requestHash := s.HashRequest(input.Request)

	if cached, ok := s.cacheStore.Get(requestHash); ok {
		log.Printf("[%s] pipeline: cache HIT hash=%s", rid, requestHash[:12])
		return ProcessOutput{StatusCode: http.StatusOK, Body: cached, FromCache: true}, nil
	}
	log.Printf("[%s] pipeline: cache MISS hash=%s, calling upstream", rid, requestHash[:12])

	start := time.Now()
	body, status, modelUsed, usage, err := s.callCloudWithFallback(ctx, rid, input.RawBody, input.Request)
	elapsed := time.Since(start)

	if err != nil {
		log.Printf("[%s] pipeline: upstream failed after %dms: %v", rid, elapsed.Milliseconds(), err)
		return ProcessOutput{}, err
	}

	log.Printf("[%s] pipeline: upstream responded status=%d model=%q tokens(in=%d out=%d) latency=%dms",
		rid, status, modelUsed, usage.PromptTokens, usage.CompletionTokens, elapsed.Milliseconds())

	if status >= 200 && status < 300 {
		s.cacheStore.Set(requestHash, body)
	}

	go s.logAudit(input, body, modelUsed, usage, int(elapsed.Milliseconds()), originalPrompt, scrubbedPrompt)

	return ProcessOutput{StatusCode: status, Body: body, FromCache: false}, nil
}

func (s *Service) callCloudWithFallback(ctx context.Context, rid string, rawBody []byte, req openai.ChatCompletionRequest) ([]byte, int, string, openai.UsageDetails, error) {
	provider := "OpenAI"
	modelInfo, err := s.repo.GetModelInfo(ctx, req.Model)
	if err != nil {
		log.Printf("[%s] provider: model registry lookup failed for model=%q, defaulting to OpenAI: %v", rid, req.Model, err)
	} else if modelInfo != nil && strings.TrimSpace(modelInfo.Provider) != "" {
		provider = modelInfo.Provider
	}
	log.Printf("[%s] provider: selected provider=%q for model=%q", rid, provider, req.Model)

	var cloudBody []byte
	var cloudStatus int
	var cloudUsage openai.UsageDetails
	var cloudErr error

	switch strings.ToLower(provider) {
	case "google", "gemini":
		log.Printf("[%s] provider: calling Gemini (Google)", rid)
		cloudBody, cloudStatus, cloudUsage, cloudErr = s.callGemini(ctx, rid, rawBody)
	case "ollama":
		log.Printf("[%s] provider: calling Ollama directly", rid)
		cloudBody, cloudUsage, cloudErr = s.callOllama(ctx, rid, req, req.Model)
		if cloudErr != nil {
			cloudStatus = http.StatusInternalServerError
		} else {
			cloudStatus = http.StatusOK
		}
	default:
		log.Printf("[%s] provider: calling OpenAI", rid)
		cloudBody, cloudStatus, cloudUsage, cloudErr = s.callOpenAI(ctx, rid, rawBody)
	}

	if cloudErr == nil && cloudStatus >= 200 && cloudStatus < 300 {
		return cloudBody, cloudStatus, req.Model, cloudUsage, nil
	}

	if cloudErr != nil {
		log.Printf("[%s] provider: cloud call error, attempting local fallback: %v", rid, cloudErr)
	} else if cloudStatus != http.StatusTooManyRequests && cloudStatus < 500 {
		log.Printf("[%s] provider: cloud returned status=%d (non-retryable), returning as-is", rid, cloudStatus)
		return cloudBody, cloudStatus, req.Model, cloudUsage, nil
	} else {
		log.Printf("[%s] provider: cloud returned status=%d, attempting local fallback. body_snippet=%s",
			rid, cloudStatus, truncate(string(cloudBody), 500))
	}

	fallbackModel, err := s.repo.LocalFallbackModel(ctx)
	if err != nil {
		log.Printf("[%s] fallback: failed to get local fallback model: %v", rid, err)
		return nil, 0, "", openai.UsageDetails{}, err
	}
	log.Printf("[%s] fallback: using local model=%q via Ollama", rid, fallbackModel)

	fallbackBody, fallbackUsage, err := s.callOllama(ctx, rid, req, fallbackModel)
	if err != nil {
		log.Printf("[%s] fallback: Ollama call failed: %v", rid, err)
		if cloudErr != nil {
			return nil, 0, "", openai.UsageDetails{}, fmt.Errorf("cloud and fallback both failed: cloud=%w fallback=%v", cloudErr, err)
		}
		return nil, 0, "", openai.UsageDetails{}, err
	}

	log.Printf("[%s] fallback: Ollama responded successfully model=%q", rid, fallbackModel)
	return fallbackBody, http.StatusOK, fallbackModel, fallbackUsage, nil
}

// callProvider is the shared HTTP call used by callOpenAI and callGemini.
func (s *Service) callProvider(ctx context.Context, rid string, rawBody []byte, endpointURL, authHeader, authValue string) ([]byte, int, openai.UsageDetails, error) {
	log.Printf("[%s] http-out: POST %s", rid, endpointURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(rawBody))
	if err != nil {
		log.Printf("[%s] http-out: failed to create request: %v", rid, err)
		return nil, 0, openai.UsageDetails{}, err
	}
	req.Header.Set(authHeader, authValue)
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("[%s] http-out: request failed after %dms: %v", rid, time.Since(start).Milliseconds(), err)
		return nil, 0, openai.UsageDetails{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[%s] http-out: failed to read response body: %v", rid, err)
		return nil, 0, openai.UsageDetails{}, err
	}

	log.Printf("[%s] http-out: response status=%d body_size=%d latency=%dms",
		rid, resp.StatusCode, len(body), time.Since(start).Milliseconds())

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[%s] http-out: non-2xx response body_snippet=%s", rid, truncate(string(body), 500))
	}

	parsed, err := openai.DecodeResponse(body)
	if err != nil {
		log.Printf("[%s] http-out: could not parse response as ChatCompletionResponse: %v", rid, err)
		return body, resp.StatusCode, openai.UsageDetails{}, nil
	}
	return body, resp.StatusCode, parsed.Usage, nil
}

func (s *Service) callOpenAI(ctx context.Context, rid string, rawBody []byte) ([]byte, int, openai.UsageDetails, error) {
	if s.cfg.OpenAIAPIKey == "" {
		log.Printf("[%s] openai: API key not configured", rid)
		return nil, 0, openai.UsageDetails{}, fmt.Errorf("OPENAI_API_KEY not configured")
	}
	endpoint := strings.TrimRight(s.cfg.OpenAIBaseURL, "/") + s.cfg.OpenAIChatPath
	return s.callProvider(ctx, rid, rawBody, endpoint, "Authorization", "Bearer "+s.cfg.OpenAIAPIKey)
}

func (s *Service) callGemini(ctx context.Context, rid string, rawBody []byte) ([]byte, int, openai.UsageDetails, error) {
	if s.cfg.GeminiAPIKey == "" {
		log.Printf("[%s] gemini: API key not configured", rid)
		return nil, 0, openai.UsageDetails{}, fmt.Errorf("GEMINI_API_KEY not configured")
	}
	// Gemini's OpenAI-compatible endpoint uses standard Bearer auth.
	endpoint := strings.TrimRight(s.cfg.GeminiBaseURL, "/") + s.cfg.GeminiChatPath
	return s.callProvider(ctx, rid, rawBody, endpoint, "Authorization", "Bearer "+s.cfg.GeminiAPIKey)
}

func (s *Service) callOllama(ctx context.Context, rid string, req openai.ChatCompletionRequest, localModel string) ([]byte, openai.UsageDetails, error) {
	log.Printf("[%s] ollama: calling url=%s model=%q", rid, s.cfg.OllamaChatURL, localModel)

	payload := openai.OllamaChatRequest{
		Model:    localModel,
		Messages: req.Messages,
		Stream:   false,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[%s] ollama: failed to marshal request: %v", rid, err)
		return nil, openai.UsageDetails{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.OllamaChatURL, bytes.NewReader(encoded))
	if err != nil {
		log.Printf("[%s] ollama: failed to create request: %v", rid, err)
		return nil, openai.UsageDetails{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("[%s] ollama: request failed after %dms: %v", rid, time.Since(start).Milliseconds(), err)
		return nil, openai.UsageDetails{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[%s] ollama: non-2xx response status=%d body_snippet=%s", rid, resp.StatusCode, truncate(string(body), 500))
		return nil, openai.UsageDetails{}, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[%s] ollama: failed to read response body: %v", rid, err)
		return nil, openai.UsageDetails{}, err
	}

	log.Printf("[%s] ollama: response status=%d body_size=%d latency=%dms",
		rid, resp.StatusCode, len(body), time.Since(start).Milliseconds())

	var ollamaResp openai.OllamaChatResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		log.Printf("[%s] ollama: failed to unmarshal response: %v", rid, err)
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

func (s *Service) logAudit(input ProcessInput, responseBody []byte, modelUsed string, usage openai.UsageDetails, latencyMS int, originalPrompt, scrubbedPrompt string) {
	responseText := extractResponseText(responseBody)

	err := s.repo.InsertAuditLog(context.Background(), db.AuditLogInput{
		AppID:          input.App.AppID,
		ModelUsed:      modelUsed,
		OriginalPrompt: originalPrompt,
		ScrubbedPrompt: scrubbedPrompt,
		ResponseText:   responseText,
		InputTokens:    usage.PromptTokens,
		OutputTokens:   usage.CompletionTokens,
		CalculatedCost: 0,
		LatencyMS:      latencyMS,
	})
	if err != nil {
		log.Printf("[%s] audit: insert failed: %v", input.RequestID, err)
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

// truncate returns at most maxLen characters of s, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
