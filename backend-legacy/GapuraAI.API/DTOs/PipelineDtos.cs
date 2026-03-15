namespace GapuraAI.API.DTOs;

/// <summary>
/// Result of the PII scrubbing step.
/// </summary>
/// <param name="ScrubbedText">The text after PII masking.</param>
/// <param name="EntitiesMasked">How many PII entities were masked.</param>
public record ScrubResult(string ScrubbedText, int EntitiesMasked);

/// <summary>
/// Result of the token counting and cost calculation step.
/// </summary>
/// <param name="InputTokens">Estimated input tokens.</param>
/// <param name="OutputTokens">Estimated output tokens.</param>
/// <param name="CalculatedCost">Total cost in USD based on Model_Registry pricing.</param>
public record TokenCostResult(int InputTokens, int OutputTokens, decimal CalculatedCost);

/// <summary>
/// Result of executing the prompt against a cloud or local model.
/// </summary>
/// <param name="Response">The OpenAI-compatible response DTO.</param>
/// <param name="ModelUsed">The model name that actually processed the request.</param>
/// <param name="UsedFallback">True if the local Ollama fallback was used.</param>
public record PipelineExecutionResult(OpenAIResponseDto Response, string ModelUsed, bool UsedFallback);
