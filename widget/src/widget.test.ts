import { afterEach, describe, expect, it, vi } from "vitest";
import { dictionaries, text } from "./locales";
import { validBridge, safeSession } from "./bridge";
import { stream, APIError } from "./transport";
afterEach(() => vi.unstubAllGlobals());
describe("widget security and localization", () => {
  it("has all four complete dictionaries", () => {
    for (const locale of ["ru", "en", "cs", "de"] as const) {
      expect(Object.keys(dictionaries[locale]).sort()).toEqual(
        Object.keys(dictionaries.en).sort(),
      );
      expect(text(locale, "demo")).toBeTruthy();
      expect(Object.keys(leadDictionaries[locale]).sort()).toEqual(
        Object.keys(leadDictionaries.en).sort(),
      );
      expect(leadText(locale, "confirm")).not.toBe("confirm");
    }
  });
  it("checks source, origin and per-instance channel", () => {
    const source = {} as Window;
    const good = {
      source,
      origin: "https://site.example",
      data: { channel: "channel", type: "init" },
    } as MessageEvent;
    expect(validBridge(good, source, good.origin, "channel")).toBe(true);
    expect(
      validBridge(
        { ...good, source: {} as Window },
        source,
        good.origin,
        "channel",
      ),
    ).toBe(false);
    expect(
      validBridge(
        { ...good, origin: "https://evil.example" },
        source,
        good.origin,
        "channel",
      ),
    ).toBe(false);
    expect(validBridge(good, source, good.origin, "other")).toBe(false);
    expect(safeSession({ id: "id", token: "x" })).toBe(false);
  });
  it("streams Unicode with bearer token outside URL", async () => {
    const bytes = new TextEncoder().encode(
      'event: delta\ndata: {"text":"Привет světe"}\n\nevent: complete\ndata: {"messages":[{"id":"1","role":"assistant","status":"completed","content":"Привет světe"}]}\n\n',
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
    let out = "";
    const result = await stream(
      "widget",
      "https://site.example",
      { id: "session", token: "secret" },
      { request_id: "request", content: "Hello" },
      new AbortController().signal,
      (s) => (out += s),
    );
    expect(out).toBe("Привет světe");
    expect(result).toHaveLength(1);
    expect(fetch.mock.calls[0][0]).not.toContain("secret");
    expect(fetch.mock.calls[0][1].headers.Authorization).toBe("Bearer secret");
    expect(fetch.mock.calls[0][1].credentials).toBe("omit");
  });
  it("rejects incomplete responses", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response('event: delta\ndata: {"text":"partial"}\n\n'),
        ),
    );
    await expect(
      stream(
        "w",
        "https://site.example",
        { id: "s", token: "x" },
        { request_id: "r", content: "hello" },
        new AbortController().signal,
        () => {},
      ),
    ).rejects.toEqual(new APIError("ai_incomplete"));
  });
});
import { leadDictionaries, leadText } from "./lead-locales";
