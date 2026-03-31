import { HttpClient, HttpHeaders, HttpParams } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import {
  AuditLogsResponse,
  ChatCompletionRequest,
  ChatCompletionResponse,
  ModelsResponse,
  StudioScorecards,
} from './models/studio.models';

@Injectable({ providedIn: 'root' })
export class StudioApiService {
  private readonly baseUrl = 'http://localhost:8080';

  constructor(private readonly http: HttpClient) {}

  getScorecards(): Observable<StudioScorecards> {
    return this.http.get<StudioScorecards>(`${this.baseUrl}/v1/studio/scorecards`);
  }

  getModels(): Observable<ModelsResponse> {
    return this.http.get<ModelsResponse>(`${this.baseUrl}/v1/studio/models`);
  }

  getAuditLogs(project: string, model: string, limit: number = 50, offset: number = 0): Observable<AuditLogsResponse> {
    let params = new HttpParams().set('limit', limit).set('offset', offset);

    if (project.trim()) {
      params = params.set('project', project.trim());
    }
    if (model.trim()) {
      params = params.set('model', model.trim());
    }

    return this.http.get<AuditLogsResponse>(`${this.baseUrl}/v1/studio/audit-logs`, {
      params,
    });
  }

  chatCompletions(
    username: string,
    password: string,
    payload: ChatCompletionRequest,
  ): Observable<ChatCompletionResponse> {
    const basic = btoa(`${username}:${password}`);
    const headers = new HttpHeaders({
      Authorization: `Basic ${basic}`,
      'Content-Type': 'application/json',
    });

    return this.http.post<ChatCompletionResponse>(
      `${this.baseUrl}/v1/chat/completions`,
      payload,
      { headers },
    );
  }
}
