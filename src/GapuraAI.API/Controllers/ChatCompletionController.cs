using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using GapuraAI.API.Data;
using GapuraAI.API.DTOs;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;

namespace GapuraAI.API.Controllers;

/// <summary>
/// Core GAPURA gateway controller.
/// Mirrors the OpenAI POST /v1/chat/completions endpoint so internal
/// applications only need to swap their BaseURL and auth header.
///
/// Mission 2 scope: straight-through proxy (no middleware/pipeline).
/// Middleware (cache, NER scrubbing, cost tracking, fallback) will be
/// injected in Mission 3 via GapuraPipelineService.
/// </summary>
[ApiController]
[Route("v1/chat")]
public class ChatCompletionController : ControllerBase
{
    private readonly GapuraDbContext _db;
    private readonly IHttpClientFactory _httpClientFactory;
    private readonly IConfiguration _configuration;
    private readonly ILogger<ChatCompletionController> _logger;

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.WhenWritingNull
    };

    public ChatCompletionController(
        GapuraDbContext db,
        IHttpClientFactory httpClientFactory,
        IConfiguration configuration,
        ILogger<ChatCompletionController> logger)
    {
        _db = db;
        _httpClientFactory = httpClientFactory;
        _configuration = configuration;
        _logger = logger;
    }

    // ───────────────────────────────────────────────────────────────────
    //  POST /v1/chat/completions
    // ───────────────────────────────────────────────────────────────────
    [HttpPost("completions")]
    [Produces("application/json")]
    public async Task<IActionResult> CreateChatCompletion(
        [FromBody] OpenAIRequestDto request,
        CancellationToken cancellationToken)
    {
        // ──── Step 1: Authenticate via Basic Auth ─────────────────────
        var authResult = await AuthenticateAsync(cancellationToken);
        if (authResult is null)
        {
            return Unauthorized(new { error = "Invalid or missing credentials." });
        }

        _logger.LogInformation(
            "Authenticated request from project '{Project}' (AppID={AppId})",
            authResult.ProjectName, authResult.AppId);

        // ──── Step 2: Forward to OpenAI (no middleware in Mission 2) ──
        var openAiApiKey = _configuration["OpenAI:ApiKey"];
        if (string.IsNullOrWhiteSpace(openAiApiKey))
        {
            _logger.LogError("OpenAI API key is not configured in appsettings.json");
            return StatusCode(500, new { error = "Gateway misconfiguration: missing upstream API key." });
        }

        var client = _httpClientFactory.CreateClient("OpenAI");

        // Serialize the incoming request to JSON
        var jsonPayload = JsonSerializer.Serialize(request, JsonOptions);
        using var httpContent = new StringContent(jsonPayload, Encoding.UTF8, "application/json");

        using var httpRequest = new HttpRequestMessage(HttpMethod.Post, "v1/chat/completions")
        {
            Content = httpContent
        };
        httpRequest.Headers.Authorization = new AuthenticationHeaderValue("Bearer", openAiApiKey);

        // ──── Step 3: Execute and stream response back ────────────────
        HttpResponseMessage upstreamResponse;
        try
        {
            upstreamResponse = await client.SendAsync(httpRequest, cancellationToken);
        }
        catch (TaskCanceledException ex) when (ex.InnerException is TimeoutException)
        {
            _logger.LogWarning("Upstream OpenAI request timed out");
            return StatusCode(504, new { error = "Upstream API timed out." });
        }
        catch (HttpRequestException ex)
        {
            _logger.LogError(ex, "Failed to reach upstream OpenAI API");
            return StatusCode(502, new { error = "Failed to reach upstream API." });
        }

        var responseBody = await upstreamResponse.Content.ReadAsStringAsync(cancellationToken);

        // If OpenAI returned an error, forward it transparently
        if (!upstreamResponse.IsSuccessStatusCode)
        {
            _logger.LogWarning(
                "Upstream returned {StatusCode}: {Body}",
                (int)upstreamResponse.StatusCode,
                responseBody.Length > 500 ? responseBody[..500] : responseBody);

            return StatusCode((int)upstreamResponse.StatusCode, JsonSerializer.Deserialize<object>(responseBody));
        }

        // ──── Step 4: Deserialize and return OpenAI response ──────────
        var openAiResponse = JsonSerializer.Deserialize<OpenAIResponseDto>(responseBody, JsonOptions);

        _logger.LogInformation(
            "Successfully proxied request for model '{Model}' — {TotalTokens} tokens",
            openAiResponse?.Model,
            openAiResponse?.Usage?.TotalTokens);

        return Ok(openAiResponse);
    }

    // ───────────────────────────────────────────────────────────────────
    //  Helper: Basic Auth  →  parse Authorization header & validate
    // ───────────────────────────────────────────────────────────────────
    private async Task<Models.AppAuth?> AuthenticateAsync(CancellationToken ct)
    {
        // Expect: Authorization: Basic <base64(username:password)>
        if (!Request.Headers.TryGetValue("Authorization", out var authHeader))
            return null;

        var headerValue = authHeader.ToString();
        if (!headerValue.StartsWith("Basic ", StringComparison.OrdinalIgnoreCase))
            return null;

        string decoded;
        try
        {
            var base64 = headerValue["Basic ".Length..].Trim();
            decoded = Encoding.UTF8.GetString(Convert.FromBase64String(base64));
        }
        catch (FormatException)
        {
            _logger.LogWarning("Malformed Base64 in Authorization header");
            return null;
        }

        var separatorIndex = decoded.IndexOf(':');
        if (separatorIndex < 0)
            return null;

        var username = decoded[..separatorIndex];
        var password = decoded[(separatorIndex + 1)..];

        // Query the Apps_Auth table for a matching user
        var appAuth = await _db.AppsAuth
            .AsNoTracking()
            .FirstOrDefaultAsync(a => a.Username == username, ct);

        if (appAuth is null)
        {
            _logger.LogWarning("Auth failed: username '{Username}' not found", username);
            return null;
        }

        // MVP: plain-text password comparison against PasswordHash column.
        // TODO (Mission 3+): Replace with BCrypt.Net.Verify() for hashed passwords.
        if (appAuth.PasswordHash != password)
        {
            _logger.LogWarning("Auth failed: incorrect password for '{Username}'", username);
            return null;
        }

        return appAuth;
    }
}
