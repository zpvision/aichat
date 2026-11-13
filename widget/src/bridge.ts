export function validBridge(
  event: MessageEvent,
  source: MessageEventSource | null,
  origin: string,
  channel: string,
) {
  return (
    event.source === source &&
    event.origin === origin &&
    event.data?.channel === channel &&
    typeof event.data?.type === "string"
  );
}
export function safeSession(
  value: unknown,
): value is { id: string; token: string } {
  const s = value as { id?: unknown; token?: unknown };
  return (
    !!s &&
    typeof s.id === "string" &&
    /^[a-f0-9-]{36}$/.test(s.id) &&
    typeof s.token === "string" &&
    /^[A-Za-z0-9_-]{43}$/.test(s.token)
  );
}
