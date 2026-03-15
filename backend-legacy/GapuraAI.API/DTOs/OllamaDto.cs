using System.Text.Json.Serialization;

namespace GapuraAI.API.DTOs;

/// <summary>
/// Request DTO for the Ollama /api/chat endpoint.
/// See: https://github.com/ollama/ollama/blob/main/docs/api.md#generate-a-chat-completion
/// </summary>
public class OllamaChatRequest
{
    [JsonPropertyName("model")]
    public string Model { get; set; } = "llama3:8b";

    [JsonPropertyName("messages")]
    public List<OllamaChatMessage> Messages { get; set; } = new();

    [JsonPropertyName("stream")]
    public bool Stream { get; set; } = false;
}

/// <summary>
/// A single message in the Ollama messages array.
/// </summary>
public class OllamaChatMessage
{
    [JsonPropertyName("role")]
    public string Role { get; set; } = string.Empty;

    [JsonPropertyName("content")]
    public string Content { get; set; } = string.Empty;
}

/// <summary>
/// Response DTO from the Ollama /api/chat endpoint (non-streaming).
/// </summary>
public class OllamaChatResponse
{
    [JsonPropertyName("model")]
    public string Model { get; set; } = string.Empty;

    [JsonPropertyName("created_at")]
    public string CreatedAt { get; set; } = string.Empty;

    [JsonPropertyName("message")]
    public OllamaChatMessage? Message { get; set; }

    [JsonPropertyName("done")]
    public bool Done { get; set; }

    [JsonPropertyName("total_duration")]
    public long TotalDuration { get; set; }

    [JsonPropertyName("prompt_eval_count")]
    public int PromptEvalCount { get; set; }

    [JsonPropertyName("eval_count")]
    public int EvalCount { get; set; }
}
