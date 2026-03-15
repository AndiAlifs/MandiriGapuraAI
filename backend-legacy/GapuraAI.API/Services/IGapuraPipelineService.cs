using GapuraAI.API.DTOs;

namespace GapuraAI.API.Services;

/// <summary>
/// Defines the GAPURA middleware pipeline that intercepts requests
/// before they reach the external/local AI model.
/// </summary>
public interface IGapuraPipelineService
{
    /// <summary>
    /// Hashes the prompt and checks for an identical cached response.
    /// Returns <c>null</c> on cache miss.
    /// </summary>
    Task<OpenAIResponseDto?> CheckCacheAsync(string promptHash);

    /// <summary>
    /// Generates a SHA-256 hash from the combined message contents.
    /// </summary>
    string HashPrompt(IEnumerable<ChatMessageDto> messages);

    /// <summary>
    /// Detects and masks Indonesian PII (NIK, account numbers) in the text.
    /// </summary>
    ScrubResult ScrubPii(string text);

    /// <summary>
    /// Simulates token counting and computes cost using Model_Registry pricing.
    /// </summary>
    Task<TokenCostResult> CountTokensAndCalculateCostAsync(
        string inputText, string outputText, string modelName);

    /// <summary>
    /// Sends the request to the cloud API. On timeout, 429, or 500,
    /// automatically retries using the local Ollama fallback.
    /// </summary>
    Task<PipelineExecutionResult> ExecuteWithFallbackAsync(
        OpenAIRequestDto request, CancellationToken ct);

    /// <summary>
    /// Persists all pipeline telemetry to the Audit_Logs table.
    /// </summary>
    Task SaveAuditLogAsync(
        int appId, string modelUsed,
        string originalPrompt, string scrubbedPrompt,
        string? responseText,
        int inputTokens, int outputTokens,
        decimal calculatedCost, int latencyMs);
}
