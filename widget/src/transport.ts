import type { Message, Session } from "./types";
export class APIError extends Error {
  constructor(public code: string) {
    super(code);
  }
}
export async function request<T>(
  widget: string,
  parent: string,
  path: string,
  method = "GET",
  session?: Session | null,
  data?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  let r: Response;
  try {
    r = await fetch("/api/v1/public/widgets/" + widget + path, {
      method,
      credentials: "omit",
      headers: {
        "Content-Type": "application/json",
        "X-Widget-Origin": parent,
        ...(session ? { Authorization: "Bearer " + session.token } : {}),
      },
      body: data === undefined ? undefined : JSON.stringify(data),
      signal,
    });
  } catch {
    if (signal?.aborted) throw new APIError("ai_canceled");
    throw new APIError("network");
  }
  const b = await r.json().catch(() => null);
  if (!r.ok) throw new APIError(b?.error?.code || "internal");
  return b;
}
export async function stream(
  widget: string,
  parent: string,
  session: Session,
  input: { request_id: string; content: string },
  signal: AbortSignal,
  onDelta: (s: string) => void,
): Promise<Message[]> {
  let response: Response;
  try {
    response = await fetch(
      "/api/v1/public/widgets/" +
        widget +
        "/sessions/" +
        session.id +
        "/messages",
      {
        method: "POST",
        credentials: "omit",
        headers: {
          "Content-Type": "application/json",
          "X-Widget-Origin": parent,
          Authorization: "Bearer " + session.token,
        },
        body: JSON.stringify(input),
        signal,
      },
    );
  } catch {
    throw new APIError(signal.aborted ? "ai_canceled" : "network");
  }
  if (!response.ok) {
    const b = await response.json().catch(() => null);
    throw new APIError(b?.error?.code || "internal");
  }
  if (!response.body) throw new APIError("ai_incomplete");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "",
    messages: Message[] | null = null;
  try {
    while (true) {
      const { value, done } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });
      let end: number;
      while ((end = buffer.indexOf("\n\n")) >= 0) {
        const block = buffer.slice(0, end);
        buffer = buffer.slice(end + 2);
        const type = block
          .split("\n")
          .find((l) => l.startsWith("event:"))
          ?.slice(6)
          .trim();
        const data = JSON.parse(
          block
            .split("\n")
            .filter((l) => l.startsWith("data:"))
            .map((l) => l.slice(5).trim())
            .join("\n"),
        );
        if (type === "delta" && typeof data.text === "string")
          onDelta(data.text);
        if (type === "error") throw new APIError(data.code || "internal");
        if (type === "complete") messages = data.messages;
      }
      if (buffer.length > 1_000_000) throw new APIError("ai_invalid_response");
      if (done) break;
    }
    if (!messages) throw new APIError("ai_incomplete");
    if (messages.some((m) => m.status === "failed" || m.status === "canceled"))
      throw new APIError("ai_incomplete");
    return messages;
  } catch (e) {
    if (signal.aborted) throw new APIError("ai_canceled");
    if (e instanceof APIError) throw e;
    throw new APIError("ai_incomplete");
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}
