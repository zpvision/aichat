// Local destructive-process test: terminates ONLY the child API it starts, never an existing server.
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";
import assert from "node:assert/strict";
const dsn = process.env.TEST_DATABASE_URL;
if (!dsn || !new URL(dsn).pathname.endsWith("_test"))
  throw Error("TEST_DATABASE_URL ending _test required");
const root = fileURLToPath(new URL("../backend/", import.meta.url));
const binary =
  process.env.INBOX_API_BINARY ||
  fileURLToPath(
    new URL(
      "../backend/" + (process.platform === "win32" ? "api.exe" : "api"),
      import.meta.url,
    ),
  );
const base = "http://localhost:18085",
  origin = "http://localhost:5173",
  parent = "http://localhost:4173";
let child,
  cookie = "";
try {
  await fetch(base + "/healthz", { signal: AbortSignal.timeout(600) });
  throw Error("Port 18085 occupied; no process will be stopped");
} catch (e) {
  if (e.message.startsWith("Port")) throw e;
}
async function start(provider) {
  child = spawn(binary, [], {
    cwd: root,
    windowsHide: true,
    stdio: "ignore",
    env: {
      ...process.env,
      APP_ENV: "development",
      DATABASE_URL: dsn,
      HTTP_ADDR: "127.0.0.1:18085",
      PUBLIC_APP_URL: origin,
      WIDGET_PUBLIC_URL: base,
      WIDGET_ALLOW_LOCALHOST: "true",
      AI_PROVIDER: provider,
      AI_TIMEOUT_SECONDS: "5",
      OLLAMA_BASE_URL: "http://localhost:11434",
      KNOWLEDGE_FILE_ROOT: "./data/knowledge-test",
      WIDGET_FILE_ROOT: "./data/widget-test",
    },
  });
  for (let i = 0; i < 40; i++) {
    if (child.exitCode !== null) throw Error("Test API exited");
    try {
      const r = await fetch(base + "/readyz", {
        signal: AbortSignal.timeout(500),
      });
      if (r.ok) return;
    } catch {}
    await delay(200);
  }
  throw Error("Test API did not start");
}
async function stop() {
  if (!child || child.exitCode !== null) return;
  const process = child;
  await new Promise((resolve) => {
    process.once("exit", resolve);
    process.kill("SIGKILL");
  });
  child = undefined;
}
async function call(path, method = "GET", body, visitor) {
  const r = await fetch(base + "/api/v1" + path, {
    method,
    headers: {
      Origin: visitor ? parent : origin,
      "Content-Type": "application/json",
      ...(visitor
        ? { Authorization: "Bearer " + visitor.token }
        : { Cookie: cookie, "X-CSRF-Protection": "1" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(10000),
  });
  if (path === "/auth/login")
    cookie = r.headers.get("set-cookie")?.split(";")[0] || "";
  assert.ok(r.ok, "Unexpected HTTP " + r.status + " at " + path);
  if (r.headers.get("content-type")?.startsWith("text/event-stream"))
    return r.text();
  return r.json();
}
try {
  await start("mock");
  const email = "restart-" + Date.now() + "@inbox.example",
    password = "restart-test-password";
  await call("/auth/register", "POST", {
    email,
    password,
    preferred_locale: "en",
  });
  await call("/auth/login", "POST", { email, password });
  const org = await call("/organizations", "POST", {
      name: "Restart fixture",
      country: "CZ",
    }),
    assistant = await call("/organizations/" + org.id + "/assistants", "POST", {
      name: "Restart guide",
    }),
    site = await call("/organizations/" + org.id + "/sites", "POST", {
      name: "Restart site",
      domain: "restart.example",
      assistant_id: assistant.id,
    });
  const admin = "/organizations/" + org.id + "/sites/" + site.id + "/widget";
  const w = await call(admin, "POST", {});
  await call(admin, "PATCH", { enabled: true, origins: [parent] });
  const pub = "/public/widgets/" + w.widget.id;
  const session = await call(pub + "/sessions", "POST", {}, {});
  const messages = pub + "/sessions/" + session.id + "/messages";
  const response = await fetch(base + "/api/v1" + messages, {
    method: "POST",
    headers: {
      Origin: parent,
      "Content-Type": "application/json",
      Authorization: "Bearer " + session.token,
    },
    body: JSON.stringify({
      request_id: crypto.randomUUID(),
      content: "Interrupt this generation",
    }),
  });
  const reader = response.body.getReader();
  const first = await reader.read();
  assert.match(new TextDecoder().decode(first.value), /event: start/);
  await stop();
  await reader.cancel().catch(() => {});
  await start("mock");
  let history;
  for (let i = 0; i < 35; i++) {
    history = await call(messages, "GET", undefined, session);
    if (
      history.messages.some(
        (m) => m.role === "assistant" && m.status === "failed",
      )
    )
      break;
    await delay(1000);
  }
  assert.ok(
    history.messages.some(
      (m) =>
        m.role === "assistant" && m.status === "failed" && m.content === "",
    ),
    "Interrupted output not recovered",
  );
  const list = await call("/organizations/" + org.id + "/conversations");
  const id = list.items[0].id,
    item = "/organizations/" + org.id + "/conversations/" + id;
  let c = list.items[0];
  c = await call(item + "/take", "POST", {
    request_id: crypto.randomUUID(),
    version: c.state_version,
  });
  await call(item + "/messages", "POST", {
    request_id: crypto.randomUUID(),
    version: c.state_version,
    content: "Staff reply after restart",
  });
  await stop();
  await start("disabled");
  const manual = await call(
    messages,
    "POST",
    { request_id: crypto.randomUUID(), content: "Visitor reply without AI" },
    session,
  );
  assert.match(manual, /event: complete/);
  history = await call(messages, "GET", undefined, session);
  assert.equal(history.control_mode, "human_active");
  assert.ok(
    history.messages.some(
      (m) => m.role === "operator" && m.content === "Staff reply after restart",
    ),
  );
  assert.ok(
    history.messages.some(
      (m) => m.role === "user" && m.content === "Visitor reply without AI",
    ),
  );
  console.log(
    "PASS: real API process crash/restart, persisted recovery/history/assignment and disabled-provider manual messages; test database only.",
  );
} finally {
  await stop();
}
