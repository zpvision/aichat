import { describe, it, expect, vi, afterEach } from "vitest";
import { SSEDecoder, streamMessage } from "./chat-api";
import { ApiError } from "./api";
afterEach(() => vi.unstubAllGlobals());
describe("SSE transport", () => {
  it("handles split events and CRLF", () => {
    const p = new SSEDecoder();
    expect(p.push("event: delta\r")).toEqual([]);
    expect(p.push('\ndata: {"text":"Čeština русский"}\r\n\r')).toEqual([]);
    expect(p.push("\n")).toEqual([
      { type: "delta", data: { text: "Čeština русский" } },
    ]);
  });
  it("decodes multilingual streaming and requires completion", async () => {
    const encoder = new TextEncoder();
    const bytes = encoder.encode(
      'event: delta\ndata: {"text":"Привет svět"}\n\nevent: complete\ndata: {"replay":false}\n\n',
    );
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        new ReadableStream({
          start(c) {
            for (let i = 0; i < bytes.length; i += 3)
              c.enqueue(bytes.slice(i, i + 3));
            c.close();
          },
        }),
        { headers: { "Content-Type": "text/event-stream" } },
      ),
    );
    vi.stubGlobal("fetch", fetch);
    const events: unknown[] = [];
    await streamMessage(
      "/test",
      { request_id: "id", content: "Hello" },
      new AbortController().signal,
      (e) => events.push(e),
    );
    expect(events).toHaveLength(2);
    expect(fetch.mock.calls[0][1].headers["X-CSRF-Protection"]).toBe("1");
    expect(fetch.mock.calls[0][1].credentials).toBe("same-origin");
  });
  it("reports incomplete streams", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response('event: delta\ndata: {"text":"partial"}\n\n', {
          headers: { "Content-Type": "text/event-stream" },
        }),
      ),
    );
    await expect(
      streamMessage(
        "/test",
        { request_id: "id", content: "x" },
        new AbortController().signal,
        () => {},
      ),
    ).rejects.toEqual(new ApiError("ai_incomplete", 502));
  });
  it("reports server error events", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response('event: error\ndata: {"code":"ai_timeout"}\n\n', {
          headers: { "Content-Type": "text/event-stream" },
        }),
      ),
    );
    await expect(
      streamMessage(
        "/test",
        { request_id: "id", content: "x" },
        new AbortController().signal,
        () => {},
      ),
    ).rejects.toEqual(new ApiError("ai_timeout", 502));
  });
});
