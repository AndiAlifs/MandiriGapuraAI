using System.Diagnostics;
using System.Net.Http.Headers;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using GapuraAI.API.Data;
using GapuraAI.API.DTOs;
using GapuraAI.API.Models;
using Microsoft.EntityFrameworkCore;

namespace GapuraAI.API.Services;

/// <summary>
/// Core middleware pipeline for the GAPURA AI Studio Gateway.
/// Intercepts every request with: caching → PII scrubbing → routing
/// with local fallback → token/cost accounting → audit logging.
/// </summary>
public class GapuraPipelineService : IGapuraPipelineService
{
    private readonly GapuraDbContext _db;
    private readonly IHttpClientFactory _httpClientFactory;
    private readonly IConfiguration _configuration;
    private readonly ILogger<GapuraPipelineService> _logger;

    // ── Regex patterns for Indonesian PII ────────────────────────────
    // NIK: exactly 16 digits (Indonesian national ID)
    private static readonly Regex NikPattern = new(
        @"\b\d{16}\b",
        RegexOptions.Compiled);

    // Bank account numbers: 10–15 digit sequences (common Indonesian bank formats)
    private static readonly Regex AccountPattern = new(
        @"\b\d{10,15}\b",
        RegexOptions.Compiled);

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.WhenWritingNull
    };

    public GapuraPipelineService(
        GapuraDbContext db,
        IHttpClientFactory httpClientFactory,
        IConfiguration configuration,
        ILogger<GapuraPipelineService> logger)
    {
        _db = db;
        _httpClientFactory = httpClientFactory;
        _configuration = configuration;
        _logger = logger;
    }

    // ═══════════════════════════════════════════════════════════════════
    //  1. CACHE CHECK
    // ═══════════════════════════════════════════════════════════════════

    /// <inheritdoc />
    public string HashPrompt(IEnumerable<ChatMessageDto> messages)
    {
        var concatenated = string.Join("|",
            messages.Select(m => $"{m.Role}:{m.Content}"));

        var hashBytes = SHA256.HashData(Encoding.UTF8.GetBytes(concatenated));
        return Convert.ToHexStringLower(hashBytes);
    }

    /// <inheritdoc />
    public Task<OpenAIResponseDto?> CheckCacheAsync(string promptHash)
    {
        // MVP: always cache miss.
        // Future: look up in Redis / IMemoryCache by promptHash within 24 hrs.
        _logger.LogDebug("Cache check for hash {Hash} — cache miss (MVP stub)", promptHash);
        return Task.FromResult<OpenAIResponseDto?>(null);
    }

    // ═══════════════════════════════════════════════════════════════════
    //  2. NER / PII SCRUBBING
    // ═══════════════════════════════════════════════════════════════════

    /// <inheritdoc />
    public ScrubResult ScrubPii(string text)
    {
        int entitiesMasked = 0;

        // Mask 16-digit NIKs first (more specific pattern)
        var scrubbed = NikPattern.Replace(text, match =>
        {
            entitiesMasked++;
            return "[NIK_MASKED]";
        });

        // Mask 10–15 digit account numbers
        scrubbed = AccountPattern.Replace(scrubbed, match =>
        {
            entitiesMasked++;
            return "[ACCOUNT_MASKED]";
        });

        _logger.LogInformation("PII scrubbing complete — {Count} entities masked", entitiesMasked);
        return new ScrubResult(scrubbed, entitiesMasked);
    }

    // ═══════════════════════════════════════════════════════════════════
    //  3. TOKEN COUNTING & COST CALCULATION
    // ═══════════════════════════════════════════════════════════════════

    /// <inheritdoc />
    public async Task<TokenCostResult> CountTokensAndCalculateCostAsync(
        string inputText, string outputText, string modelName)
    {
        // Simulated tokenizer: ~4 chars per token
        int inputTokens = Math.Max(1, inputText.Length / 4);
        int outputTokens = Math.Max(1, outputText.Length / 4);

        // Look up the model's pricing from the registry
        var model = await _db.ModelRegistry
            .AsNoTracking()
            .FirstOrDefaultAsync(m => m.ModelName == modelName);

        decimal cost = 0m;
        if (model is not null)
        {
            cost = (inputTokens / 1000m * model.CostPer1kInput)
                 + (outputTokens / 1000m * model.CostPer1kOutput);
        }
        else
        {
            _logger.LogWarning(
                "Model '{ModelName}' not found in Model_Registry — cost defaulting to 0",
                modelName);
        }

        _logger.LogInformation(
            "Token count: in={In}, out={Out}, cost=${Cost:F6} (model={Model})",
            inputTokens, outputTokens, cost, modelName);

        return new TokenCostResult(inputTokens, outputTokens, cost);
    }

    // ═══════════════════════════════════════════════════════════════════
    //  4. EXECUTE WITH LOCAL FALLBACK
    // ═══════════════════════════════════════════════════════════════════

    /// <inheritdoc />
    public async Task<PipelineExecutionResult> ExecuteWithFallbackAsync(
        OpenAIRequestDto request, CancellationToken ct)
    {
        // ── Resolve the provider from the Model_Registry ─────────────
        var registeredModel = await _db.ModelRegistry
            .AsNoTracking()
            .FirstOrDefaultAsync(m => m.ModelName == request.Model, ct);

        var provider = registeredModel?.Provider ?? "OpenAI"; // default to OpenAI

        _logger.LogInformation(
            "Routing model '{Model}' via provider '{Provider}'",
            request.Model, provider);

        // ── Try the resolved cloud provider first ────────────────────
        try
        {
            var cloudResponse = provider.Equals("Gemini", StringComparison.OrdinalIgnoreCase)
                ? await CallGeminiAsync(request, ct)
                : await CallOpenAiAsync(request, ct);

            return new PipelineExecutionResult(cloudResponse, request.Model, UsedFallback: false);
        }
        catch (Exception ex) when (
            ex is TaskCanceledException ||
            ex is HttpRequestException ||
            ex is CloudApiErrorException)
        {
            _logger.LogWarning(
                ex, "Cloud API ({Provider}) failed ({ExType}). Falling back to local Ollama…",
                provider, ex.GetType().Name);
        }

        // ── Fallback to local Ollama ─────────────────────────────────
        _logger.LogInformation("Routing to local Ollama fallback");

        var fallbackModel = await _db.ModelRegistry
            .AsNoTracking()
            .FirstOrDefaultAsync(m => m.IsLocalFallback, ct);

        var localModelName = fallbackModel?.ModelName ?? "llama3:8b";

        var ollamaResponse = await CallOllamaAsync(request, localModelName, ct);
        return new PipelineExecutionResult(ollamaResponse, localModelName, UsedFallback: true);
    }

    // ── OpenAI cloud call with 10-second timeout ─────────────────────
    private async Task<OpenAIResponseDto> CallOpenAiAsync(
        OpenAIRequestDto request, CancellationToken ct)
    {
        var apiKey = _configuration["OpenAI:ApiKey"]
            ?? throw new InvalidOperationException("OpenAI:ApiKey is not configured.");

        var client = _httpClientFactory.CreateClient("OpenAI");

        var jsonPayload = JsonSerializer.Serialize(request, JsonOptions);
        using var httpContent = new StringContent(jsonPayload, Encoding.UTF8, "application/json");

        using var httpRequest = new HttpRequestMessage(HttpMethod.Post, "v1/chat/completions")
        {
            Content = httpContent
        };
        httpRequest.Headers.Authorization = new AuthenticationHeaderValue("Bearer", apiKey);

        return await SendCloudRequestAsync(client, httpRequest, "OpenAI", ct);
    }

    // ── Gemini cloud call (OpenAI-compatible endpoint) ───────────────
    private async Task<OpenAIResponseDto> CallGeminiAsync(
        OpenAIRequestDto request, CancellationToken ct)
    {
        var apiKey = _configuration["Gemini:ApiKey"]
            ?? throw new InvalidOperationException("Gemini:ApiKey is not configured.");

        var client = _httpClientFactory.CreateClient("Gemini");

        var jsonPayload = JsonSerializer.Serialize(request, JsonOptions);
        using var httpContent = new StringContent(jsonPayload, Encoding.UTF8, "application/json");

        using var httpRequest = new HttpRequestMessage(HttpMethod.Post, "chat/completions")
        {
            Content = httpContent
        };
        // Gemini uses x-goog-api-key header instead of Bearer token
        httpRequest.Headers.Add("x-goog-api-key", apiKey);

        return await SendCloudRequestAsync(client, httpRequest, "Gemini", ct);
    }

    // ── Shared cloud request execution with timeout & error handling ─
    private async Task<OpenAIResponseDto> SendCloudRequestAsync(
        HttpClient client, HttpRequestMessage httpRequest,
        string providerName, CancellationToken ct)
    {
        // Enforce a 10 second timeout per-request via a linked CTS
        using var timeoutCts = new CancellationTokenSource(TimeSpan.FromSeconds(10));
        using var linkedCts = CancellationTokenSource.CreateLinkedTokenSource(ct, timeoutCts.Token);

        var response = await client.SendAsync(httpRequest, linkedCts.Token);

        // Treat 429 (rate limit) and 5xx as fallback-worthy errors
        if ((int)response.StatusCode == 429 ||
            (int)response.StatusCode >= 500)
        {
            var body = await response.Content.ReadAsStringAsync(ct);
            throw new CloudApiErrorException(
                $"{providerName} returned {(int)response.StatusCode}: {body[..Math.Min(body.Length, 200)]}");
        }

        var responseBody = await response.Content.ReadAsStringAsync(ct);
        return JsonSerializer.Deserialize<OpenAIResponseDto>(responseBody, JsonOptions)
            ?? throw new InvalidOperationException($"Failed to deserialize {providerName} response.");
    }

    // ── Local Ollama call ────────────────────────────────────────────
    private async Task<OpenAIResponseDto> CallOllamaAsync(
        OpenAIRequestDto request, string localModelName, CancellationToken ct)
    {
        var client = _httpClientFactory.CreateClient("Ollama");

        // Translate OpenAI message format → Ollama message format
        var ollamaRequest = new OllamaChatRequest
        {
            Model = localModelName,
            Stream = false,
            Messages = request.Messages
                .Select(m => new OllamaChatMessage { Role = m.Role, Content = m.Content })
                .ToList()
        };

        var jsonPayload = JsonSerializer.Serialize(ollamaRequest, JsonOptions);
        using var httpContent = new StringContent(jsonPayload, Encoding.UTF8, "application/json");

        var response = await client.PostAsync("api/chat", httpContent, ct);
        response.EnsureSuccessStatusCode();

        var responseBody = await response.Content.ReadAsStringAsync(ct);
        var ollamaResponse = JsonSerializer.Deserialize<OllamaChatResponse>(responseBody, JsonOptions)
            ?? throw new InvalidOperationException("Failed to deserialize Ollama response.");

        // Translate Ollama response → OpenAI-compatible response DTO
        var assistantContent = ollamaResponse.Message?.Content ?? string.Empty;
        var estimatedPromptTokens = ollamaResponse.PromptEvalCount > 0
            ? ollamaResponse.PromptEvalCount
            : request.Messages.Sum(m => m.Content.Length) / 4;
        var estimatedCompletionTokens = ollamaResponse.EvalCount > 0
            ? ollamaResponse.EvalCount
            : assistantContent.Length / 4;

        return new OpenAIResponseDto
        {
            Id = $"chatcmpl-ollama-{Guid.NewGuid():N}",
            Object = "chat.completion",
            Created = DateTimeOffset.UtcNow.ToUnixTimeSeconds(),
            Model = localModelName,
            Choices = new List<ChoiceDto>
            {
                new()
                {
                    Index = 0,
                    Message = new ChatMessageDto
                    {
                        Role = "assistant",
                        Content = assistantContent
                    },
                    FinishReason = "stop"
                }
            },
            Usage = new UsageDto
            {
                PromptTokens = estimatedPromptTokens,
                CompletionTokens = estimatedCompletionTokens,
                TotalTokens = estimatedPromptTokens + estimatedCompletionTokens
            }
        };
    }

    // ═══════════════════════════════════════════════════════════════════
    //  5. AUDIT LOGGING
    // ═══════════════════════════════════════════════════════════════════

    /// <inheritdoc />
    public async Task SaveAuditLogAsync(
        int appId, string modelUsed,
        string originalPrompt, string scrubbedPrompt,
        string? responseText,
        int inputTokens, int outputTokens,
        decimal calculatedCost, int latencyMs)
    {
        var log = new AuditLog
        {
            AppId = appId,
            ModelUsed = modelUsed,
            OriginalPrompt = originalPrompt,
            ScrubbedPrompt = scrubbedPrompt,
            ResponseText = responseText,
            InputTokens = inputTokens,
            OutputTokens = outputTokens,
            CalculatedCost = calculatedCost,
            LatencyMs = latencyMs,
            Timestamp = DateTime.UtcNow
        };

        _db.AuditLogs.Add(log);
        await _db.SaveChangesAsync();

        _logger.LogInformation(
            "Audit log saved (LogId={LogId}, AppId={AppId}, Model={Model}, Cost=${Cost:F6}, Latency={Ms}ms)",
            log.LogId, appId, modelUsed, calculatedCost, latencyMs);
    }
}

// ─────────────────────────────────────────────────────────────────────
//  Internal exception used to signal fallback-worthy cloud errors
// ─────────────────────────────────────────────────────────────────────
/// <summary>
/// Thrown when the cloud API returns a status code (429 / 5xx) that
/// warrants automatic fallback to the local model.
/// </summary>
public class CloudApiErrorException : Exception
{
    public CloudApiErrorException(string message) : base(message) { }
}
