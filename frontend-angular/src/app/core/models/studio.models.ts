export interface ModelRegistryItem {
  modelID: number;
  modelName: string;
  provider: string;
  costPer1kInput: number;
  costPer1kOutput: number;
  isLocalFallback: boolean;
}

export interface StudioScorecards {
  total_pii_entities_scrubbed: number;
  total_api_cost_saved: number;
}

export interface AuditLogItem {
  log_id: number;
  app_id: number;
  project_name: string;
  model_used: string;
  original_prompt: string;
  scrubbed_prompt: string;
  response_text: string;
  input_tokens: number;
  output_tokens: number;
  calculated_cost: number;
  latency_ms: number;
  timestamp: string;
}

export interface AuditLogsResponse {
  items: AuditLogItem[];
  limit: number;
  offset: number;
}

export interface ModelsResponse {
  items: ModelRegistryItem[];
}

export interface ChatMessage {
  role: 'system' | 'user' | 'assistant';
  content: string;
}

export interface ChatCompletionRequest {
  model: string;
  messages: ChatMessage[];
  stream: boolean;
}

export interface ChatCompletionResponse {
  id: string;
  model: string;
  choices: Array<{
    index: number;
    message: {
      role: string;
      content: string;
    };
    finish_reason: string;
  }>;
  usage?: {
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
  };
}
