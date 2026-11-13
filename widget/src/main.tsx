import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { validBridge, safeSession } from "./bridge";
import {
  type Config,
  type Locale,
  type Session,
  type Message,
  locales,
  uuid,
} from "./types";
import { text, languageNames, type Key } from "./locales";
import { request, stream, APIError } from "./transport";
import "./style.css";
import { LeadCard } from "./LeadCard";
const params = new URLSearchParams(location.search);
const parentOrigin = params.get("parent_origin") || "";
const channel = params.get("channel") || "";
const id = location.pathname.split("/").filter(Boolean).at(-1) || "";
const preview = id === "preview";
function mergeMessages(old: Message[], incoming: Message[]) {
  const map = new Map(old.map((m) => [m.id, m]));
  for (const m of incoming) {
    if ((map.get(m.id)?.change_seq || 0) > (m.change_seq || 0)) continue;
    map.set(m.id, m);
  }
  return Array.from(map.values()).sort((a, b) => a.sequence - b.sequence);
}
function Icon({ name }: { name: "chat" | "close" | "send" | "new" | "stop" }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
      {name === "chat" ? (
        <>
          <path d="M20 11.5a8 8 0 0 1-8 8H5l-3 2V11.5a9 9 0 1 1 18 0Z" />
          <path d="M7 10h8M7 14h5" />
        </>
      ) : name === "close" ? (
        <path d="m6 6 12 12M18 6 6 18" />
      ) : name === "send" ? (
        <>
          <path d="m4 11 16-8-7 17-2-7-7-2Z" />
          <path d="m11 13 9-10" />
        </>
      ) : name === "stop" ? (
        <rect x="6" y="6" width="12" height="12" rx="2" />
      ) : (
        <>
          <path d="M12 5v14M5 12h14" />
        </>
      )}
    </svg>
  );
}
function App() {
  const [cfg, setCfg] = useState<Config | null>(null);
  const [locale, setLocale] = useState<Locale>("en");
  const [messages, setMessages] = useState<Message[]>([]);
  const [draft, setDraft] = useState("");
  const [partial, setPartial] = useState("");
  const [pending, setPending] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirmNew, setConfirmNew] = useState(false);
  const [loading, setLoading] = useState(!preview);
  const [ready, setReady] = useState(false);
  const [mode, setMode] = useState("ai_active");
  const [olderBefore, setOlderBefore] = useState(0),
    [olderBusy, setOlderBusy] = useState(false);
  const [events, setEvents] = useState<
    { id: string; kind: string; created_at: string; sequence: number }[]
  >([]);
  const cursor = useRef(0),
    shown = useRef(true),
    polling = useRef(false);
  const modeVersion = useRef(0);
  const session = useRef<Session | null>(null);
  const [sessionId, setSessionId] = useState("");
  const abort = useRef<AbortController | null>(null);
  const generationAI = useRef(false);
  const retry = useRef<{ request_id: string; content: string } | null>(null);
  const list = useRef<HTMLDivElement | null>(null);
  const oldScroll = useRef<{ height: number; top: number } | null>(null);
  const input = useRef<HTMLTextAreaElement | null>(null);
  const dialog = useRef<HTMLElement | null>(null);
  const t = (key: Key) => text(locale, key);
  const post = (type: string, data?: unknown) =>
    window.parent.postMessage({ type, channel, data }, parentOrigin);
  function store(s: Session | null) {
    cursor.current = 0;
    setOlderBefore(0);
    modeVersion.current = 0;
    setMode("ai_active");
    setEvents([]);
    session.current = s;
    setSessionId(s?.id || "");
    post("session", s);
  }
  function problem(e: unknown) {
    const code = e instanceof APIError ? e.code : "network";
    setError(code);
    if (code === "visitor_unauthorized") {
      store(null);
      setMessages([]);
    }
  }
  async function history(s: Session, incremental = false, before = 0) {
    const d = await request<{
      messages: Message[];
      events: {
        id: string;
        kind: string;
        created_at: string;
        sequence: number;
      }[];
      control_mode: string;
      state_version: number;
      cursor: number;
      next_before: number;
      more: boolean;
    }>(
      id,
      parentOrigin,
      "/sessions/" +
        s.id +
        "/messages" +
        (before
          ? "?before=" + before
          : incremental && cursor.current
            ? "?after=" + cursor.current
            : ""),
      "GET",
      s,
    );
    if (session.current?.id !== s.id) return [];
    cursor.current = Math.max(cursor.current, d.cursor);
    if (!incremental) setOlderBefore(d.more ? d.next_before : 0);
    const newest = d.state_version >= modeVersion.current;
    if (newest) {
      modeVersion.current = d.state_version;
      setMode(d.control_mode);
    }
    if (
      newest &&
      (d.control_mode === "closed" ||
        (generationAI.current && d.control_mode !== "ai_active"))
    )
      abort.current?.abort();
    setEvents((old) =>
      Array.from(
        new Map([...old, ...d.events].map((e) => [e.id, e])).values(),
      ).sort((a, b) => a.sequence - b.sequence),
    );
    const incoming =
      abort.current && d.control_mode === "ai_active"
        ? d.messages.filter((m) => m.request_id !== retry.current?.request_id)
        : d.messages;
    setMessages((old) => mergeMessages(old, incoming));
    return d.messages;
  }
  async function earlier() {
    const s = session.current;
    if (!s || olderBusy) return;
    setOlderBusy(true);
    try {
      if (list.current)
        oldScroll.current = {
          height: list.current.scrollHeight,
          top: list.current.scrollTop,
        };
      await history(s, false, olderBefore);
    } catch (e) {
      oldScroll.current = null;
      problem(e);
    } finally {
      setOlderBusy(false);
    }
  }
  useEffect(() => {
    const tick = async () => {
      const s = session.current;
      if (!s || preview || !shown.current || document.hidden || polling.current)
        return;
      polling.current = true;
      try {
        await history(s, true);
      } catch (e) {
        problem(e);
      } finally {
        polling.current = false;
      }
    };
    const timer = setInterval(() => void tick(), 3000);
    document.addEventListener("visibilitychange", tick);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", tick);
    };
  }, []);
  async function requestHuman() {
    if (preview || mode === "closed") return;
    setError("");
    try {
      let s = session.current;
      if (!s) {
        s = await request<Session>(
          id,
          parentOrigin,
          "/sessions",
          "POST",
          null,
          {},
        );
        store(s);
      }
      await request(
        id,
        parentOrigin,
        "/sessions/" + s.id + "/request-human",
        "POST",
        s,
        { request_id: crypto.randomUUID() },
      );
      abort.current?.abort();
      await history(s, true);
    } catch (e) {
      problem(e);
    }
  }
  async function stop() {
    const s = session.current;
    const active = abort.current;
    if (s && active) {
      void request(
        id,
        parentOrigin,
        "/sessions/" + s.id + "/cancel",
        "POST",
        s,
        {},
      ).catch(() => {});
      active.abort();
    }
  }
  useEffect(() => {
    let mounted = true;
    async function receive(event: MessageEvent) {
      if (!validBridge(event, window.parent, parentOrigin, channel)) return;
      if (event.data.type === "preview" && preview) {
        const c = event.data.data.config as Config;
        setCfg(c);
        setLocale(event.data.data.locale || c.settings.default_locale);
        setLoading(false);
        setReady(true);
        return;
      }
      if (event.data.type === "init" && !preview) {
        if (!uuid.test(id)) return;
        try {
          const c = await request<Config>(id, parentOrigin, "/config");
          if (!mounted) return;
          setCfg(c);
          const lang = c.settings.auto_locale
            ? navigator.language.split("-")[0]
            : c.settings.default_locale;
          setLocale(
            locales.includes(lang as Locale)
              ? (lang as Locale)
              : c.settings.default_locale,
          );
          if (
            c.settings.remember_session &&
            safeSession(event.data.data?.session)
          ) {
            session.current = event.data.data.session;
            setSessionId(session.current!.id);
            try {
              await history(session.current!);
            } catch (e) {
              problem(e);
            }
          }
          setReady(true);
        } catch (e) {
          problem(e);
        } finally {
          setLoading(false);
        }
      }
      if (event.data.type === "visibility") {
        shown.current = !!event.data.data?.open;
        if (!event.data.data?.open) {
          void stop();
        } else {
          const s = session.current;
          if (s) void history(s).catch(problem);
          setTimeout(() => input.current?.focus(), 80);
        }
      }
    }
    window.addEventListener("message", receive);
    post("ready");
    return () => {
      mounted = false;
      window.removeEventListener("message", receive);
      abort.current?.abort();
    };
  }, []);
  useEffect(() => {
    if (!confirmNew) return;
    const buttons =
      dialog.current?.querySelectorAll<HTMLButtonElement>("button");
    buttons?.[0]?.focus();
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setConfirmNew(false);
        input.current?.focus();
      }
      if (e.key === "Tab" && buttons?.length) {
        e.preventDefault();
        const next =
          document.activeElement === buttons[0] ? buttons[1] : buttons[0];
        next.focus();
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [confirmNew]);
  useEffect(() => {
    if (ready && !preview) input.current?.focus();
  }, [ready]);
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);
  useEffect(() => {
    const el = input.current;
    if (el) {
      el.style.height = "auto";
      el.style.height = Math.min(el.scrollHeight, 120) + "px";
    }
  }, [draft]);
  useEffect(() => {
    if (list.current) {
      list.current.scrollTop = oldScroll.current
        ? list.current.scrollHeight -
          oldScroll.current.height +
          oldScroll.current.top
        : list.current.scrollHeight;
      oldScroll.current = null;
    }
  }, [messages, partial, pending, error]);
  async function send() {
    if (
      preview ||
      busy ||
      abort.current ||
      !draft.trim() ||
      !cfg?.available ||
      mode === "closed"
    )
      return;
    const content = draft;
    const payload =
      retry.current?.content === content
        ? retry.current
        : { request_id: crypto.randomUUID(), content };
    retry.current = payload;
    const controller = new AbortController();
    generationAI.current = mode === "ai_active" && cfg.provider !== "disabled";
    abort.current = controller;
    setBusy(true);
    setError("");
    setPending(content);
    setPartial("");
    setDraft("");
    try {
      let s = session.current;
      if (!s) {
        s = await request<Session>(
          id,
          parentOrigin,
          "/sessions",
          "POST",
          null,
          {},
          controller.signal,
        );
        store(s);
      }
      const received = await stream(
        id,
        parentOrigin,
        s,
        payload,
        controller.signal,
        (value) => setPartial((p) => p + value),
      );
      setMessages((m) => mergeMessages(m, received));
      retry.current = null;
    } catch (e) {
      problem(e);
      setDraft(content);
      const s = session.current;
      if (s)
        try {
          const found = await history(s);
          const reply = found.find(
            (m) =>
              m.request_id === payload.request_id && m.role === "assistant",
          );
          if (reply?.status === "completed") {
            setError("");
            setDraft("");
            retry.current = null;
          } else if (reply) {
            setDraft("");
            retry.current = null;
          } else if (
            found.some(
              (m) => m.role === "user" && m.request_id === payload.request_id,
            ) &&
            (mode !== "ai_active" || cfg.provider === "disabled")
          ) {
            setError("");
            setDraft("");
            retry.current = null;
          }
        } catch {}
    } finally {
      setBusy(false);
      setPending("");
      setPartial("");
      abort.current = null;
      generationAI.current = false;
      input.current?.focus();
    }
  }
  const settings = cfg?.settings;
  const own =
    settings?.texts[locale] || settings?.texts[settings.default_locale];
  const title = own?.title || t("title");
  const greeting = own?.greeting || t("greeting");
  const shade = (hex: string) => {
    let rgb = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
    while (luminance(rgb) > 0.12) rgb = rgb.map((x) => Math.round(x * 0.95));
    return "#" + rgb.map((x) => x.toString(16).padStart(2, "0")).join("");
  };
  const vars = settings
    ? ({
        "--primary": settings.primary,
        "--header-a": shade(settings.primary),
        "--header-b": shade(settings.secondary),
        "--user": settings.user_color,
        "--user-ink":
          luminance(
            [1, 3, 5].map((i) =>
              parseInt(settings.user_color.slice(i, i + 2), 16),
            ),
          ) > 0.179
            ? "#14233b"
            : "#ffffff",
      } as React.CSSProperties)
    : {};
  const errorKey: Key = [
    "widget_rate",
    "widget_capacity",
    "ai_busy",
    "conversation_busy",
    "request_in_progress",
  ].includes(error)
    ? "busy"
    : error === "widget_session_limit"
      ? "limit"
      : error === "visitor_unauthorized"
        ? "expired"
        : error === "ai_canceled"
          ? "canceled"
          : ["ai_incomplete", "ai_invalid_response"].includes(error)
            ? "failed"
            : error === "network"
              ? "network"
              : "unavailable";
  return (
    <main className="chat" style={vars}>
      <header className="chat-header">
        <div className="header-top">
          <div className="identity">
            {cfg?.logo_url ? (
              <img src={cfg.logo_url} alt="" />
            ) : (
              <span className="chat-symbol">
                <Icon name="chat" />
              </span>
            )}
            <span className="virtual">
              {t(
                mode === "human_active"
                  ? "staff"
                  : mode === "human_requested"
                    ? "waiting"
                    : "badge",
              )}
            </span>
          </div>
          <div className="header-actions">
            {settings?.allow_new && (
              <button
                title={t("new")}
                aria-label={t("new")}
                disabled={busy || preview}
                onClick={() => setConfirmNew(true)}
              >
                <Icon name="new" />
              </button>
            )}
            <button
              title={t("close")}
              aria-label={t("close")}
              onClick={() => {
                void stop();
                post("close");
              }}
            >
              <Icon name="close" />
            </button>
          </div>
        </div>
        <h1>{title}</h1>
        <p>{own?.subtitle || t("subtitle")}</p>
      </header>
      {cfg?.demo && (
        <div className="demo" role="status">
          <span />
          {t("demo")}
        </div>
      )}
      {preview && <div className="preview-label">{t("preview")}</div>}
      {mode !== "ai_active" && (
        <div className="channel-state" role="status">
          {t(
            mode === "closed"
              ? "closed"
              : mode === "human_requested"
                ? "requested"
                : "humanActive",
          )}
        </div>
      )}
      {cfg?.provider === "disabled" && mode === "ai_active" && (
        <div className="channel-state">{t("manualAvailable")}</div>
      )}
      <div className="messages" ref={list} aria-live="polite">
        {olderBefore > 0 && (
          <button
            className="request-human"
            type="button"
            disabled={olderBusy || busy}
            onClick={() => void earlier()}
          >
            {t("older")}
          </button>
        )}
        <div className="welcome">
          <span className="welcome-icon">
            <Icon name="chat" />
          </span>
          <div>
            <span className="speaker">{title}</span>
            <p>{greeting}</p>
          </div>
        </div>
        {loading && (
          <p className="muted" role="status">
            {t("loading")}
          </p>
        )}
        {cfg && !cfg.available && <p className="notice">{t("unavailable")}</p>}
        {messages.map((m) => (
          <article
            key={m.id}
            className={"message " + m.role}
            role={
              m.status === "processing"
                ? "status"
                : m.status !== "completed"
                  ? "alert"
                  : undefined
            }
            aria-label={t(
              m.role === "user"
                ? "you"
                : m.role === "operator"
                  ? "staff"
                  : "assistant",
            )}
          >
            {m.role === "operator" && (
              <span className="speaker">{t("staff")}</span>
            )}
            <MessageText
              value={
                m.content ||
                t(
                  m.status === "processing"
                    ? "generating"
                    : m.status === "canceled"
                      ? "canceled"
                      : "failed",
                )
              }
            />
            {m.content && m.status !== "completed" && (
              <small>
                {t(m.status === "canceled" ? "canceled" : "failed")}
              </small>
            )}
          </article>
        ))}
        {pending && (
          <>
            <article className="message user">
              <MessageText value={pending} />
            </article>
            {generationAI.current && (
              <article className="message assistant partial">
                {partial ? (
                  <MessageText value={partial} />
                ) : (
                  <div
                    className="typing"
                    role="status"
                    aria-label={t("generating")}
                  >
                    <i />
                    <i />
                    <i />
                  </div>
                )}
              </article>
            )}
          </>
        )}
        {error &&
          !(
            error === "ai_canceled" &&
            messages.some((m) => m.status === "canceled")
          ) && (
            <div
              className={"notice " + (error === "ai_canceled" ? "neutral" : "")}
              role="alert"
            >
              {t(errorKey)}
            </div>
          )}
        {!preview && (cfg?.lead_enabled || sessionId) && (
          <LeadCard
            enabled={!!cfg?.lead_enabled}
            widgetId={id}
            parentOrigin={parentOrigin}
            locale={locale}
            sessionId={sessionId}
            getSession={() => session.current}
            isVisible={() => shown.current}
            disabled={busy || mode === "closed"}
            mode={mode}
            ensureSession={async () => {
              if (session.current) return session.current;
              const s = await request<Session>(
                id,
                parentOrigin,
                "/sessions",
                "POST",
                null,
                {},
              );
              store(s);
              return s;
            }}
          />
        )}
      </div>
      <footer className="composer-area">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void send();
          }}
        >
          <div className="composer">
            <textarea
              ref={input}
              aria-label={t("placeholder")}
              placeholder={t("placeholder")}
              rows={1}
              value={draft}
              maxLength={cfg?.max_message_chars || 2000}
              disabled={
                preview ||
                !ready ||
                !cfg?.available ||
                busy ||
                mode === "closed"
              }
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (
                  e.key === "Enter" &&
                  !e.shiftKey &&
                  !e.nativeEvent.isComposing
                ) {
                  e.preventDefault();
                  void send();
                }
              }}
            />
            {busy && generationAI.current ? (
              <button
                type="button"
                className="send stop"
                title={t("stop")}
                aria-label={t("stop")}
                onClick={() => void stop()}
              >
                <Icon name="stop" />
              </button>
            ) : (
              <button
                type="button"
                onClick={() => void send()}
                className="send"
                title={t("send")}
                aria-label={t("send")}
                disabled={
                  preview ||
                  busy ||
                  !draft.trim() ||
                  !ready ||
                  !cfg?.available ||
                  mode === "closed"
                }
              >
                <Icon name="send" />
              </button>
            )}
          </div>
          <div className="composer-meta">
            <select
              aria-label={t("language")}
              value={locale}
              onChange={(e) => setLocale(e.target.value as Locale)}
            >
              {locales.map((l) => (
                <option key={l} value={l}>
                  {languageNames[l]}
                </option>
              ))}
            </select>
            {!preview && mode === "ai_active" && (
              <button
                type="button"
                className="request-human"
                onClick={() => void requestHuman()}
              >
                {t("requestHuman")}
              </button>
            )}
            {settings?.privacy_url ? (
              <a
                href={settings.privacy_url}
                target="_blank"
                rel="noopener noreferrer"
              >
                {t("privacy")}
              </a>
            ) : (
              <span>{t("footer")}</span>
            )}
          </div>
        </form>
      </footer>
      {events.length > 0 && (
        <div className="channel-events" aria-live="polite">
          {t(
            events.at(-1)!.kind === "ai_active"
              ? "aiReturned"
              : events.at(-1)!.kind === "closed"
                ? "closed"
                : events.at(-1)!.kind === "human_active"
                  ? "humanActive"
                  : "requested",
          )}
        </div>
      )}
      {confirmNew && (
        <div className="confirm">
          <section
            ref={dialog}
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="new-title"
          >
            <h2 id="new-title">{t("newTitle")}</h2>
            <p>{t("newBody")}</p>
            <button onClick={() => setConfirmNew(false)}>{t("back")}</button>
            <button
              className="primary"
              onClick={() => {
                store(null);
                setMessages([]);
                retry.current = null;
                setDraft("");
                setError("");
                setConfirmNew(false);
                input.current?.focus();
              }}
            >
              {t("confirm")}
            </button>
          </section>
        </div>
      )}
    </main>
  );
}
function luminance(rgb: number[]) {
  const v = rgb.map((x) => {
    x /= 255;
    return x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4;
  });
  return v[0] * 0.2126 + v[1] * 0.7152 + v[2] * 0.0722;
}
function MessageText({ value }: { value: string }) {
  const parts = value.split(/(https?:\/\/[^\s<>]+)/g);
  return (
    <p>
      {parts.map((part, i) =>
        /^https?:\/\//.test(part) ? (
          <a key={i} href={part} target="_blank" rel="noopener noreferrer">
            {part}
          </a>
        ) : (
          part
        ),
      )}
    </p>
  );
}
if (
  (uuid.test(id) || preview) &&
  uuid.test(channel) &&
  parentOrigin &&
  window.parent !== window
)
  createRoot(document.getElementById("root")!).render(<App />);
