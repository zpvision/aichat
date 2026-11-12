import { ApiError } from "./api";
export type AISettings = {
  display_name: string;
  role: string;
  company_description: string;
  style: string;
  instructions: string;
  no_answer: string;
  language_policy: "auto" | "fixed" | "default";
  response_locale: string;
  offer_operator: boolean;
};
export type ProviderSettings = {
  settings: AISettings;
  provider: string;
  model: string;
  demo: boolean;
  max_message_chars: number;
};
export type Conversation = {
  id: string;
  created_at: string;
  updated_at: string;
  status: string;
};
export type ChatMessage = {
  id: string;
  execution_id: string;
  sequence: number;
  role: string;
  content: string;
  language: string;
  status: string;
  created_at: string;
};
export type Source = {
  chunk_id: string;
  document_id: string;
  title: string;
  content: string;
  url: string;
  page: number | null;
  index_version: number;
  model: string;
  model_version: string;
};
export type Execution = {
  id: string;
  request_id: string;
  provider: string;
  model: string;
  status: string;
  error_code: string;
  sources: Source[];
  duration_ms: number;
  input_tokens: number | null;
  output_tokens: number | null;
  policy_fallback: boolean;
};
export type Detail = {
  conversation: Conversation;
  messages: ChatMessage[];
  executions: Execution[];
  next_before: number | null;
};
export function mergeDetail(previous: Detail | null, incoming: Detail): Detail {
  if (
    !previous ||
    previous.conversation.id !== incoming.conversation.id ||
    previous.messages.length <= 100
  )
    return incoming;
  const messages = new Map(previous.messages.map((m) => [m.id, m]));
  for (const m of incoming.messages) messages.set(m.id, m);
  const executions = new Map(previous.executions.map((e) => [e.id, e]));
  for (const e of incoming.executions) executions.set(e.id, e);
  return {
    ...incoming,
    messages: [...messages.values()].sort((a, b) => a.sequence - b.sequence),
    executions: [...executions.values()],
    next_before: previous.next_before,
  };
}
export type ChatResult = {
  execution: Execution;
  user_message: ChatMessage;
  assistant_message: ChatMessage;
  replay: boolean;
};
export type StreamEvent = { type: string; data: any };
export class SSEDecoder {
  private buffer = "";
  push(text: string): StreamEvent[] {
    this.buffer = (this.buffer + text).replace(/\r\n/g, "\n");
    const events: StreamEvent[] = [];
    let end: number;
    while ((end = this.buffer.indexOf("\n\n")) >= 0) {
      const block = this.buffer.slice(0, end);
      this.buffer = this.buffer.slice(end + 2);
      let type = "message";
      const data: string[] = [];
      for (const line of block.split("\n")) {
        if (line.startsWith("event:")) type = line.slice(6).trim();
        if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
      }
      if (data.length) events.push({ type, data: JSON.parse(data.join("\n")) });
    }
    if (this.buffer.length > 2_000_000)
      throw new ApiError("ai_invalid_response", 502);
    return events;
  }
}
export async function streamMessage(
  path: string,
  input: { request_id: string; content: string },
  signal: AbortSignal,
  onEvent: (e: StreamEvent) => void,
) {
  let response: Response;
  try {
    response = await fetch("/api/v1" + path, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-CSRF-Protection": "1" },
      body: JSON.stringify(input),
      signal,
    });
  } catch (e) {
    if (signal.aborted) throw new ApiError("ai_canceled", 409);
    throw new ApiError("network", 0);
  }
  if (!response.ok) {
    const b = await response.json().catch(() => null);
    throw new ApiError(b?.error?.code || "internal", response.status);
  }
  if (
    !response.body ||
    !response.headers.get("Content-Type")?.includes("text/event-stream")
  )
    throw new ApiError("ai_invalid_response", 502);
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const parser = new SSEDecoder();
  let complete = false;
  try {
    while (true) {
      const { value, done } = await reader.read();
      const events = parser.push(decoder.decode(value, { stream: !done }));
      for (const event of events) {
        if (event.type === "error")
          throw new ApiError(event.data.code || "internal", 502);
        if (event.type === "complete") complete = true;
        onEvent(event);
      }
      if (done) break;
    }
    if (!complete) throw new ApiError("ai_incomplete", 502);
  } catch (e) {
    if (signal.aborted) throw new ApiError("ai_canceled", 409);
    if (e instanceof ApiError) throw e;
    throw new ApiError("ai_incomplete", 502);
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}
