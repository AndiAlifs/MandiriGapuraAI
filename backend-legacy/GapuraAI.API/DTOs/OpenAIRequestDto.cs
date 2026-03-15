using System.Text.Json.Serialization;

namespace GapuraAI.API.DTOs;

/// <summary>
/// DTO mirroring the standard OpenAI Chat Completion request payload.
/// See: https://platform.openai.com/docs/api-reference/chat/create
/// </summary>
public class OpenAIRequestDto
{
    [JsonPropertyName("model")]
    public string Model { get; set; } = string.Empty;

    [JsonPropertyName("messages")]
    public List<ChatMessageDto> Messages { get; set; } = new();

    [JsonPropertyName("temperature")]
    public float? Temperature { get; set; }

    [JsonPropertyName("max_tokens")]
    public int? MaxTokens { get; set; }

    [JsonPropertyName("top_p")]
    public float? TopP { get; set; }

    [JsonPropertyName("stream")]
    public bool? Stream { get; set; }
}

/// <summary>
/// Represents a single message in the OpenAI messages array.
/// </summary>
public class ChatMessageDto
{
    [JsonPropertyName("role")]
    public string Role { get; set; } = string.Empty;

    [JsonPropertyName("content")]
    public string Content { get; set; } = string.Empty;
}
