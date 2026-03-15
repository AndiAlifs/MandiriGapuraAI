using System.Text.Json.Serialization;

namespace GapuraAI.API.DTOs;

/// <summary>
/// DTO mirroring the standard OpenAI Chat Completion response payload.
/// See: https://platform.openai.com/docs/api-reference/chat/object
/// </summary>
public class OpenAIResponseDto
{
    [JsonPropertyName("id")]
    public string Id { get; set; } = string.Empty;

    [JsonPropertyName("object")]
    public string Object { get; set; } = "chat.completion";

    [JsonPropertyName("created")]
    public long Created { get; set; }

    [JsonPropertyName("model")]
    public string Model { get; set; } = string.Empty;

    [JsonPropertyName("choices")]
    public List<ChoiceDto> Choices { get; set; } = new();

    [JsonPropertyName("usage")]
    public UsageDto? Usage { get; set; }
}

/// <summary>
/// A single completion choice returned by the model.
/// </summary>
public class ChoiceDto
{
    [JsonPropertyName("index")]
    public int Index { get; set; }

    [JsonPropertyName("message")]
    public ChatMessageDto Message { get; set; } = new();

    [JsonPropertyName("finish_reason")]
    public string? FinishReason { get; set; }
}

/// <summary>
/// Token usage statistics for the completion request.
/// </summary>
public class UsageDto
{
    [JsonPropertyName("prompt_tokens")]
    public int PromptTokens { get; set; }

    [JsonPropertyName("completion_tokens")]
    public int CompletionTokens { get; set; }

    [JsonPropertyName("total_tokens")]
    public int TotalTokens { get; set; }
}
