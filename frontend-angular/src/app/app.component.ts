import { CommonModule } from '@angular/common';
import { Component, OnInit } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { StudioApiService } from './core/studio-api.service';
import {
  AuditLogItem,
  ChatCompletionResponse,
  ModelRegistryItem,
  StudioScorecards,
} from './core/models/studio.models';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './app.component.html',
  styleUrl: './app.component.css'
})
export class AppComponent implements OnInit {
  username = '';
  password = '';
  selectedModel = 'gpt-4o-mini';
  systemPrompt = 'You are a helpful banking assistant.';
  userPrompt = 'Summarize transaction fraud indicators in 3 bullets.';

  projectFilter = '';
  modelFilter = '';
  auditLimit = 10;
  auditOffset = 0;
  hasMoreAuditLogs = false;

  scorecards: StudioScorecards = {
    total_pii_entities_scrubbed: 0,
    total_api_cost_saved: 0,
  };
  models: ModelRegistryItem[] = [];
  auditLogs: AuditLogItem[] = [];

  selectedLogId: number | null = null;

  playgroundLoading = false;
  playgroundError = '';
  assistantResponse = '';
  promptTokens = 0;
  completionTokens = 0;

  estimatedInputTokens = 0;
  simulatedCosts: Array<{ model: string; total: number }> = [];

  curlSnippet = '';
  goSnippet = '';

  selectedSnippetLang = 'curl';
  copyFeedback = '';

  constructor(private readonly api: StudioApiService) {}

  copySnippet(): void {
    const snippet = this.selectedSnippetLang === 'curl' ? this.curlSnippet : this.goSnippet;
    navigator.clipboard.writeText(snippet).then(() => {
      this.copyFeedback = 'Copied!';
      setTimeout(() => (this.copyFeedback = ''), 2000);
    }).catch(() => {
      this.copyFeedback = 'Failed';
      setTimeout(() => (this.copyFeedback = ''), 2000);
    });
  }

  ngOnInit(): void {
    this.refreshModels();
    this.refreshScorecards();
    this.refreshAuditLogs();
    this.recalculateEstimates();
    this.generateCodeSnippets();
  }

  refreshModels(): void {
    this.api.getModels().subscribe({
      next: (res) => {
        this.models = res.items ?? [];
        if (this.models.length > 0 && !this.models.some((m) => m.modelName === this.selectedModel)) {
          this.selectedModel = this.models[0].modelName;
        }
        this.recalculateEstimates();
      },
      error: () => {
        this.models = [];
      },
    });
  }

  refreshScorecards(): void {
    this.api.getScorecards().subscribe({
      next: (res) => {
        this.scorecards = res;
      },
      error: () => {
        this.scorecards = {
          total_pii_entities_scrubbed: 0,
          total_api_cost_saved: 0,
        };
      },
    });
  }

  refreshAuditLogs(): void {
    this.auditOffset = 0;
    this.fetchAuditLogs();
  }

  fetchAuditLogs(): void {
    this.api.getAuditLogs(this.projectFilter, this.modelFilter, this.auditLimit, this.auditOffset).subscribe({
      next: (res) => {
        this.auditLogs = res.items ?? [];
        this.hasMoreAuditLogs = this.auditLogs.length === this.auditLimit;
      },
      error: () => {
        this.auditLogs = [];
        this.hasMoreAuditLogs = false;
      },
    });
  }

  nextAuditPage(): void {
    if (this.hasMoreAuditLogs) {
      this.auditOffset += this.auditLimit;
      this.fetchAuditLogs();
    }
  }

  prevAuditPage(): void {
    if (this.auditOffset >= this.auditLimit) {
      this.auditOffset -= this.auditLimit;
      this.fetchAuditLogs();
    }
  }

  runPlayground(): void {
    this.playgroundError = '';
    this.assistantResponse = '';
    this.promptTokens = 0;
    this.completionTokens = 0;

    if (!this.username.trim() || !this.password.trim()) {
      this.playgroundError = 'Username and password are required to call /v1/chat/completions.';
      return;
    }

    if (!this.userPrompt.trim()) {
      this.playgroundError = 'Prompt cannot be empty.';
      return;
    }

    this.playgroundLoading = true;
    const messages = [];
    if (this.systemPrompt.trim()) {
      messages.push({ role: 'system' as const, content: this.systemPrompt.trim() });
    }
    messages.push({ role: 'user' as const, content: this.userPrompt.trim() });

    this.api
      .chatCompletions(this.username, this.password, {
        model: this.selectedModel,
        messages,
        stream: false,
      })
      .subscribe({
        next: (res: ChatCompletionResponse) => {
          this.assistantResponse = res.choices?.[0]?.message?.content ?? '';
          this.promptTokens = res.usage?.prompt_tokens ?? this.estimateTokens(this.systemPrompt + ' ' + this.userPrompt);
          this.completionTokens = res.usage?.completion_tokens ?? this.estimateTokens(this.assistantResponse);
          this.recalculateEstimates();
          this.refreshScorecards();
          this.refreshAuditLogs();
          this.playgroundLoading = false;
        },
        error: (err) => {
          this.playgroundError = err?.error?.error ?? 'Failed to execute prompt in playground.';
          this.playgroundLoading = false;
        },
      });
  }

  recalculateEstimates(): void {
    this.estimatedInputTokens = this.estimateTokens(`${this.systemPrompt}\n${this.userPrompt}`);
    const outputTokens = this.completionTokens > 0 ? this.completionTokens : Math.max(24, Math.floor(this.estimatedInputTokens * 0.35));

    this.simulatedCosts = this.models.map((model) => {
      const inputCost = (this.estimatedInputTokens / 1000) * model.costPer1kInput;
      const outputCost = (outputTokens / 1000) * model.costPer1kOutput;
      return {
        model: model.modelName,
        total: inputCost + outputCost,
      };
    });

    this.generateCodeSnippets();
  }

  generateCodeSnippets(): void {
    const sanitizedPrompt = this.userPrompt.replace(/\"/g, '\\\"');
    const endpoint = 'http://localhost:8080/v1/chat/completions';
    const authPair = `${this.username || 'YOUR_USERNAME'}:${this.password || 'YOUR_PASSWORD'}`;
    const basic = btoa(authPair);

    this.curlSnippet = [
      `curl -X POST ${endpoint} \\\\`,
      `  -H \"Authorization: Basic ${basic}\" \\\\`,
      '  -H "Content-Type: application/json" \\\\',
      '  -d "{',
      `    \\\"model\\\": \\\"${this.selectedModel}\\\",`,
      '    \\\"messages\\\": [',
      `      {\\\"role\\\": \\\"user\\\", \\\"content\\\": \\\"${sanitizedPrompt || 'YOUR_PROMPT'}\\\"}`,
      '    ],',
      '    \\\"stream\\\": false',
      '  }"',
    ].join('\n');

    this.goSnippet = [
      'package main',
      '',
      'import (',
      '  "bytes"',
      '  "fmt"',
      '  "net/http"',
      ')',
      '',
      'func main() {',
      `  payload := []byte(\`{\"model\":\"${this.selectedModel}\",\"messages\":[{\"role\":\"user\",\"content\":\"${sanitizedPrompt || 'YOUR_PROMPT'}\"}],\"stream\":false}\`)`,
      `  req, _ := http.NewRequest("POST", "${endpoint}", bytes.NewBuffer(payload))`,
      `  req.Header.Set("Authorization", "Basic ${basic}")`,
      '  req.Header.Set("Content-Type", "application/json")',
      '  resp, err := http.DefaultClient.Do(req)',
      '  if err != nil { panic(err) }',
      '  defer resp.Body.Close()',
      '  fmt.Println("Status:", resp.Status)',
      '}',
    ].join('\n');
  }

  toggleAuditRow(logID: number): void {
    this.selectedLogId = this.selectedLogId === logID ? null : logID;
  }

  isAuditExpanded(logID: number): boolean {
    return this.selectedLogId === logID;
  }

  private estimateTokens(text: string): number {
    const normalized = text.trim();
    if (!normalized) {
      return 0;
    }
    return Math.max(1, Math.ceil(normalized.length / 4));
  }
}
