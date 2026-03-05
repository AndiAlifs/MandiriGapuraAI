# GAPURA AI Studio: Agent Mission Prompts

## 🚀 The Initialization Prompt
*Paste this along with the PRD and Technical Reference into a fresh AI chat window to set the context before giving any mission prompts.*

Act as a Senior Enterprise Architect and Full-Stack Developer. I am building the **GAPURA AI Studio MVP**. 
Read the following Product Requirements Document (PRD) and Technical Reference Guide. 
Acknowledge that you understand the stack (.NET 8, Angular 18, MySQL) and the strict MVP scope. Do not write any code yet. Just reply 'Context accepted. Ready for Mission 1.' when you are ready.

**[PASTE PRD HERE]**

**[PASTE TECHNICAL REFERENCE HERE]**

---

## 🛠️ Mission 1 Prompt: Database & Foundation Layer
*Use this to generate the exact database files and C# models.*

We are executing **Mission 1: The Foundation Layer**. 

Please generate the following complete, production-ready files:
1. `init.sql`: A MySQL initialization script that creates the `Apps_Auth`, `Model_Registry`, and `Audit_Logs` tables exactly as defined in the Technical Reference. Include `INSERT` statements to seed one dummy user (Project: 'PopCorn RAG') and two models ('gpt-4o-mini' as Cloud, 'llama3-8b-local' as Local Fallback).
2. `GapuraDbContext.cs`: The Entity Framework Core DbContext mapping to these tables.
3. The three C# Entity Model classes (`AppAuth.cs`, `ModelRegistry.cs`, `AuditLog.cs`) using standard EF Core data annotations matching the MySQL types.

Ensure the C# code targets .NET 8. Output the code in clean markdown blocks. Do not omit any properties.

---

## 🔌 Mission 2 Prompt: The OpenAI API Proxy
*Use this to generate the core routing controller.*

We are executing **Mission 2: The OpenAI API Proxy**. 

Please generate the `ChatCompletionController.cs` for our .NET 8 Web API. 

Requirements:
1. Create a `POST /v1/chat/completions` endpoint.
2. Create the necessary C# DTO classes (`OpenAIRequestDto`, `OpenAIResponseDto`) to strictly accept and return the exact JSON structure of the standard OpenAI API.
3. Implement a basic authentication check: read the `Authorization` header, query the `GapuraDbContext` for a matching user, and return `401 Unauthorized` if not found.
4. Inject an `HttpClient` to forward the incoming JSON payload to the real `https://api.openai.com/v1/chat/completions`. 
5. For now, bypass the middleware logic (we will add that in Mission 3). Just map the request, forward it, and return the response. Ensure async/await best practices.

---

## 🧠 Mission 3 Prompt: The Middleware Engine (The Brains)
*Use this to generate the complex scrubbing, cost tracking, and fallback logic.*

We are executing **Mission 3: The Middleware Engine**. 

Please generate a `GapuraPipelineService.cs` that intercepts the request in the controller before it hits the external API.

Implement the following methods sequentially:
1. **Cache Check:** Hash the prompt. (Return a dummy cache miss for now).
2. **NER Scrubbing:** Write a Regex method that finds and masks 16-digit Indonesian NIKs (replace with `[NIK_MASKED]`) and common account number patterns in the prompt string.
3. **Token & Cost Math:** Write a method that simulates token counting (e.g., string length / 4) and calculates the cost based on the `Model_Registry` pricing.
4. **The Fallback Wrapper:** Wrap the `HttpClient` call in a `try/catch`. Set a 10-second timeout. If it catches an exception (or gets a 429/500), automatically change the URI to `http://localhost:11434/api/chat` (our local VPS Ollama) and retry.
5. **Audit Logging:** Write the async method to save the `OriginalPrompt`, `ScrubbedPrompt`, `CalculatedCost`, and `LatencyMS` to the `AuditLog` table using EF Core.

Provide the complete service class and show how to inject it into the Controller from Mission 2.

---

## 🎨 Mission 4 Prompt: The Developer Experience (Angular Playground)
*Use this to build the frontend chat interface and snippet generator.*

We are executing **Mission 4: The Developer Experience**. 

Please generate the Angular 18 (Standalone Components) code for the **Model Playground**. I am using Tailwind CSS for styling.

Generate the following:
1. `playground.component.ts` & `.html`: A UI split into two columns. Left column: Input textarea for the prompt and a dropdown to select the Model. Right column: The chat response output area.
2. **The Snippet Generator Modal:** Add a 'Get Code' button. When clicked, it opens a modal displaying two code blocks (C# `HttpClient` and `cURL`). These code blocks must dynamically inject the user's prompt, selected model, and the `http://localhost:5000/v1/chat/completions` gateway URL.
3. `gapura-api.service.ts`: An Angular HTTP service to connect the playground to our .NET backend. 

Make the UI look like a modern, dark-mode enterprise developer tool.

---

## 📊 Mission 5 Prompt: The Executive Dashboards
*Use this to generate the backend analytics queries and the frontend data tables.*

We are executing **Mission 5: The Executive Dashboards**.

**Part 1 (Backend):** Generate an `AnalyticsController.cs` with two endpoints using EF Core:
- `GET /api/analytics/scorecards`: Returns the sum of total requests, total cost saved (where cost was 0 due to local fallback), and a simulated count of PII entities scrubbed.
- `GET /api/analytics/logs`: Returns the top 50 most recent `Audit_Logs` descending by timestamp.

**Part 2 (Frontend):** Generate the Angular 18 components for the Dashboard:
- `dashboard.component.ts` & `.html`: Display the scorecards using large metric cards.
- `audit-grid.component.ts` & `.html`: A data table to display the logs. Crucially, implement a 'row expand' feature so clicking a log row reveals a hidden div showing the `OriginalPrompt` vs. `ScrubbedPrompt`. 

Provide clean, modular code.

---

## 🖥️ Mission 6 Prompt: The Local VPS Ops Setup
*Use this to have the AI generate the exact bash script for your server.*

We are executing **Mission 6: The Local VPS Engine**.

I am running an Ubuntu VPS with 8GB VRAM. Please generate a step-by-step terminal guide and a combined Bash script to:
1. Install Ollama.
2. Pull the `llama3:8b` (or most efficient quantized model for an 8GB VRAM limit) model.
3. Configure the Ollama service to expose its API on `0.0.0.0:11434` so my .NET gateway can access it locally.
4. Provide a sample `curl` command I can run in my terminal to verify the local AI is responding to chat completion requests.