using System.Diagnostics;
using System.Text;
using System.Text.Json;
using GapuraAI.API.Data;
using GapuraAI.API.DTOs;
using GapuraAI.API.Services;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;

namespace GapuraAI.API.Controllers;

/// <summary>
/// Core GAPURA gateway controller.
/// Mirrors the OpenAI POST /v1/chat/completions endpoint so internal
/// applications only need to swap their BaseURL and auth header.
///
/// Mission 3: Full pipeline integration — cache, NER scrubbing,
/// token/cost math, Ollama fallback, and audit logging.
/// </summary>
[ApiController]
[Route("v1/chat")]
public class ChatCompletionController : ControllerBase
{
    private readonly GapuraDbContext _db;
    private readonly IGapuraPipelineService _pipeline;
    private readonly IConfiguration _configuration;
    private readonly ILogger<ChatCompletionController> _logger;

    public ChatCompletionController(
        GapuraDbContext db,
        IGapuraPipelineService pipeline,
        IConfiguration configuration,
        ILogger<ChatCompletionController> logger)
    {
        _db = db;
        _pipeline = pipeline;
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
        var stopwatch = Stopwatch.StartNew();

        // ──── Step 1: Authenticate via Basic Auth ─────────────────────
        var authResult = await AuthenticateAsync(cancellationToken);
        if (authResult is null)
        {
            return Unauthorized(new { error = "Invalid or missing credentials." });
        }

        _logger.LogInformation(
            "Authenticated request from project '{Project}' (AppID={AppId})",
            authResult.ProjectName, authResult.AppId);

        // ──── Step 2: Capture original prompt & scrub PII ─────────────
        var originalPrompt = string.Join("\n",
            request.Messages.Select(m => $"{m.Role}: {m.Content}"));

        var scrubResult = _pipeline.ScrubPii(originalPrompt);

        // Apply scrubbed text back into the request messages for forwarding
        foreach (var msg in request.Messages)
        {
            msg.Content = _pipeline.ScrubPii(msg.Content).ScrubbedText;
        }

        // ──── Step 3: Cache check ─────────────────────────────────────
        var promptHash = _pipeline.HashPrompt(request.Messages);
        var cached = await _pipeline.CheckCacheAsync(promptHash);
        if (cached is not null)
        {
            _logger.LogInformation("Cache hit for hash {Hash}", promptHash);
            stopwatch.Stop();
            return Ok(cached);
        }

        // ──── Step 4: Execute with fallback ───────────────────────────
        PipelineExecutionResult execResult;
        try
        {
            execResult = await _pipeline.ExecuteWithFallbackAsync(
                request, cancellationToken);
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "All upstream providers (cloud + local) failed");
            return StatusCode(502, new { error = "All upstream providers failed." });
        }

        if (execResult.UsedFallback)
        {
            _logger.LogInformation("Request served by local fallback model '{Model}'",
                execResult.ModelUsed);
        }

        // ──── Step 5: Token counting & cost calculation ───────────────
        var responseText = execResult.Response.Choices.FirstOrDefault()?.Message.Content ?? "";
        var scrubbedPrompt = string.Join("\n",
            request.Messages.Select(m => $"{m.Role}: {m.Content}"));

        var costResult = await _pipeline.CountTokensAndCalculateCostAsync(
            scrubbedPrompt, responseText, execResult.ModelUsed);

        // ──── Step 6: Audit logging (fire-and-forget) ─────────────────
        stopwatch.Stop();
        var latencyMs = (int)stopwatch.ElapsedMilliseconds;

        _ = Task.Run(async () =>
        {
            try
            {
                await _pipeline.SaveAuditLogAsync(
                    appId: authResult.AppId,
                    modelUsed: execResult.ModelUsed,
                    originalPrompt: originalPrompt,
                    scrubbedPrompt: scrubbedPrompt,
                    responseText: responseText,
                    inputTokens: costResult.InputTokens,
                    outputTokens: costResult.OutputTokens,
                    calculatedCost: costResult.CalculatedCost,
                    latencyMs: latencyMs);
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Failed to save audit log");
            }
        }, CancellationToken.None);

        // ──── Step 7: Return response ─────────────────────────────────
        _logger.LogInformation(
            "Request complete — model={Model}, tokens(in={In},out={Out}), cost=${Cost:F6}, latency={Ms}ms, fallback={Fallback}",
            execResult.ModelUsed,
            costResult.InputTokens, costResult.OutputTokens,
            costResult.CalculatedCost, latencyMs, execResult.UsedFallback);

        return Ok(execResult.Response);
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
        // TODO: Replace with BCrypt.Net.Verify() for hashed passwords.
        if (appAuth.PasswordHash != password)
        {
            _logger.LogWarning("Auth failed: incorrect password for '{Username}'", username);
            return null;
        }

        return appAuth;
    }
}
