import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, locales, type Assistant } from "./api";
import {
  streamMessage,
  mergeDetail,
  type AISettings,
  type ProviderSettings,
  type Conversation,
  type Detail,
  type ChatMessage,
  type Execution,
  type ChatResult,
} from "./chat-api";
export function PreviewChat({
  organizationId,
  assistants,
  writable,
  initialAssistant,
}: {
  organizationId: string;
  assistants: Assistant[];
  writable: boolean;
  initialAssistant: string;
}) {
  const { t, i18n } = useTranslation();
  const [assistant, setAssistant] = useState(
    initialAssistant || assistants[0]?.id || "",
  );
  const [configuration, setConfiguration] = useState<ProviderSettings | null>(
    null,
  );
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [selected, setSelected] = useState("");
  const [detail, setDetail] = useState<Detail | null>(null);
  const [question, setQuestion] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [streaming, setStreaming] = useState("");
  const [sending, setSending] = useState("");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const controller = useRef<AbortController | null>(null);
  const retry = useRef<{
    id: string;
    conversation: string;
    content: string;
  } | null>(null);
  const base = "/organizations/" + organizationId + "/assistants/" + assistant;
  const conversationBase = base + "/preview/conversations";
  const fail = (e: unknown) =>
    setError(e instanceof ApiError ? e.code : "internal");
  useEffect(() => {
    if (!assistant) {
      setLoading(false);
      return;
    }
    let active = true;
    setConfiguration(null);
    setSelected("");
    setDetail(null);
    setError("");
    setLoading(true);
    setQuestion("");
    setSettingsOpen(false);
    void Promise.all([
      api<ProviderSettings>(base + "/ai/settings"),
      api<Conversation[]>(conversationBase),
    ])
      .then(([s, c]) => {
        if (active) {
          setConfiguration(s);
          setConversations(c);
          setSelected(c[0]?.id || "");
        }
      })
      .catch((e) => {
        if (active) fail(e);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
      controller.current?.abort();
      controller.current = null;
    };
  }, [base, conversationBase]);
  useEffect(() => {
    if (!selected) {
      setDetail(null);
      return;
    }
    let active = true;
    const load = () =>
      api<Detail>(conversationBase + "/" + selected)
        .then((d) => {
          if (active) setDetail((previous) => mergeDetail(previous, d));
        })
        .catch((e) => {
          if (active) fail(e);
        });
    void load();
    const timer = setInterval(() => {
      if (!controller.current) void load();
    }, 3000);
    return () => {
      active = false;
      clearInterval(timer);
      controller.current?.abort();
      controller.current = null;
      retry.current = null;
    };
  }, [conversationBase, selected]);
  async function act(fn: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      fail(e);
    } finally {
      setBusy(false);
    }
  }
  async function newConversation() {
    await act(async () => {
      const c = await api<Conversation>(conversationBase, "POST", {});
      setConversations((prev) => [c, ...prev]);
      setSelected(c.id);
      setQuestion("");
    });
  }
  async function send() {
    if (controller.current || !selected || !question.trim() || !writable)
      return;
    const content = question;
    const current = selected;
    const request =
      retry.current?.conversation === current &&
      retry.current.content === content
        ? retry.current
        : { id: crypto.randomUUID(), conversation: current, content };
    retry.current = request;
    const abort = new AbortController();
    controller.current = abort;
    setBusy(true);
    setError("");
    setSending(content);
    setStreaming("");
    setQuestion("");
    try {
      await streamMessage(
        conversationBase + "/" + current + "/messages/stream",
        { request_id: request.id, content },
        abort.signal,
        (event) => {
          if (event.type === "delta")
            setStreaming((prev) => prev + event.data.text);
          if (event.type === "complete") {
            const result = event.data as ChatResult;
            retry.current = null;
            if (result.execution.status !== "completed")
              throw new ApiError(
                result.execution.error_code || "ai_incomplete",
                502,
              );
          }
        },
      );
    } catch (e) {
      fail(e);
      if (!abort.signal.aborted) setQuestion(content);
    } finally {
      if (controller.current === abort) {
        try {
          const d = await api<Detail>(conversationBase + "/" + current);
          setDetail((previous) => mergeDetail(previous, d));
          const execution = d.executions.find(
            (e) => e.request_id === request.id,
          );
          if (execution?.status === "completed") {
            retry.current = null;
            setQuestion("");
            setError("");
          }
          if (
            execution?.status === "failed" ||
            execution?.status === "canceled"
          )
            retry.current = null;
        } catch (e) {
          fail(e);
        }
        setBusy(false);
        setSending("");
        setStreaming("");
        controller.current = null;
      }
    }
  }
  async function cancel() {
    const active = controller.current;
    if (!selected) return;
    try {
      await api(conversationBase + "/" + selected + "/cancel", "POST", {});
      if (!active)
        setDetail(await api<Detail>(conversationBase + "/" + selected));
    } catch (e) {
      fail(e);
    } finally {
      active?.abort();
    }
  }
  const date = (s: string) =>
    new Intl.DateTimeFormat(i18n.resolvedLanguage || "en", {
      dateStyle: "short",
      timeStyle: "short",
    }).format(new Date(s));
  const execution = (m: ChatMessage) =>
    detail?.executions.find((e) => e.id === m.execution_id);
  const generating =
    !!sending || !!detail?.executions.some((e) => e.status === "processing");
  function sources(e: Execution | undefined) {
    return (
      e &&
      e.sources?.length > 0 && (
        <details className="chat-sources">
          <summary>
            {t("chat.sources")} ({e.sources.length})
          </summary>
          {e.sources.map((s) => (
            <article key={s.chunk_id}>
              <strong>{s.title}</strong>
              <p className="description">{s.content}</p>
              {s.url && (
                <a href={s.url} target="_blank" rel="noopener noreferrer">
                  {s.url}
                </a>
              )}
              <small>
                {s.page ? `${t("kb.page")} ${s.page} · ` : ""}
                {t("kb.version")} {s.index_version} · {s.model}
              </small>
            </article>
          ))}
        </details>
      )
    );
  }
  async function older() {
    if (!detail?.next_before) return;
    await act(async () => {
      const d = await api<Detail>(
        conversationBase + "/" + selected + "?before=" + detail.next_before,
      );
      setDetail((prev) =>
        prev
          ? {
              ...prev,
              messages: [...d.messages, ...prev.messages],
              executions: [...d.executions, ...prev.executions].filter(
                (v, i, a) => a.findIndex((x) => x.id === v.id) === i,
              ),
              next_before: d.next_before,
            }
          : d,
      );
    });
  }
  return (
    <section className="preview-chat">
      <label className="assistant-picker">
        {t("assistant")}
        <select
          aria-label={t("assistant")}
          value={assistant}
          disabled={busy}
          onChange={(e) => setAssistant(e.target.value)}
        >
          <option value="" disabled>
            {t("selectAssistant")}
          </option>
          {assistants.map((a) => (
            <option value={a.id} key={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      </label>
      {!assistant ? (
        <section className="empty card">{t("createAssistantFirst")}</section>
      ) : (
        <>
          {configuration && (
            <section className="chat-provider card">
              <strong>
                {t("chat.provider")}: {configuration.provider}{" "}
                {configuration.model && `· ${configuration.model}`}
              </strong>
              <p
                className={configuration.demo ? "info" : "muted"}
                role="status"
              >
                {t(
                  configuration.demo
                    ? "chat.demo"
                    : configuration.provider === "disabled"
                      ? "chat.disabled"
                      : "chat.ollamaConfigured",
                )}
              </p>
              {writable && (
                <button
                  disabled={busy}
                  onClick={() => setSettingsOpen((v) => !v)}
                >
                  {t("chat.settings")}
                </button>
              )}
            </section>
          )}
          {error && (
            <div className="alert" role="alert">
              {t("errors." + error, { defaultValue: t("errors.internal") })}
              <button aria-label={t("close")} onClick={() => setError("")}>
                ×
              </button>
            </div>
          )}
          {loading && <p role="status">{t("loading")}</p>}
          {settingsOpen && configuration && (
            <AISettingsForm
              key={assistant}
              settings={configuration.settings}
              busy={busy}
              onSave={(settings) =>
                void act(async () => {
                  setConfiguration(
                    await api<ProviderSettings>(
                      base + "/ai/settings",
                      "PATCH",
                      settings,
                    ),
                  );
                  setSettingsOpen(false);
                })
              }
            />
          )}
          <div className="chat-toolbar">
            <label>
              {t("chat.conversation")}
              <select
                aria-label={t("chat.conversation")}
                value={selected}
                disabled={busy}
                onChange={(e) => setSelected(e.target.value)}
              >
                <option value="">{t("chat.selectConversation")}</option>
                {conversations.map((c) => (
                  <option value={c.id} key={c.id}>
                    {date(c.created_at)} · {c.id.slice(0, 8)}
                  </option>
                ))}
              </select>
            </label>
            {writable && (
              <>
                <button
                  className="primary"
                  disabled={busy}
                  onClick={() => void newConversation()}
                >
                  {t("chat.new")}
                </button>
                {selected && (
                  <button
                    className="danger"
                    disabled={busy}
                    onClick={() => setDeleteOpen(true)}
                  >
                    {t("delete")}
                  </button>
                )}
              </>
            )}
          </div>
          {!selected ? (
            <section className="empty card">
              <h2>{t("chat.empty")}</h2>
              <p>{t("chat.emptyHint")}</p>
            </section>
          ) : (
            <section className="card chat-panel">
              <h2>
                {configuration?.settings.display_name ||
                  assistants.find((a) => a.id === assistant)?.name}
              </h2>
              {detail?.next_before && (
                <button disabled={busy} onClick={() => void older()}>
                  {t("chat.older")}
                </button>
              )}
              <div className="chat-messages" aria-live="polite">
                {detail?.messages.map((m) => (
                  <article key={m.id} className={"chat-message " + m.role}>
                    <strong>{t("chat.roles." + m.role)}</strong>
                    <p className="description">
                      {m.content || t("chat.messageStatus." + m.status)}
                    </p>
                    <small>
                      {date(m.created_at)} ·{" "}
                      {t("chat.messageStatus." + m.status)}
                    </small>
                    {m.role === "assistant" && (
                      <>
                        {execution(m)?.error_code && (
                          <p className="form-error">
                            {t("errors." + execution(m)!.error_code, {
                              defaultValue: t("errors.internal"),
                            })}
                          </p>
                        )}
                        {sources(execution(m))}
                      </>
                    )}
                  </article>
                ))}
                {sending && (
                  <>
                    <article className="chat-message user">
                      <strong>{t("chat.roles.user")}</strong>
                      <p className="description">{sending}</p>
                    </article>
                    <article className="chat-message assistant">
                      <strong>{t("chat.roles.assistant")}</strong>
                      <p className="description">
                        {streaming || t("chat.generating")}
                      </p>
                    </article>
                  </>
                )}
              </div>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void send();
                }}
              >
                <label>
                  {t("chat.message")}
                  <textarea
                    aria-label={t("chat.message")}
                    value={question}
                    onChange={(e) => setQuestion(e.target.value)}
                    maxLength={configuration?.max_message_chars || 2000}
                    disabled={
                      busy ||
                      generating ||
                      !writable ||
                      configuration?.provider === "disabled"
                    }
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
                </label>
                <div className="chat-send">
                  <small>{t("chat.enterHint")}</small>
                  {controller.current || (generating && writable) ? (
                    <button
                      type="button"
                      className="danger"
                      onClick={() => void cancel()}
                    >
                      {t("chat.cancel")}
                    </button>
                  ) : (
                    <button
                      className="primary"
                      disabled={
                        busy ||
                        generating ||
                        !question.trim() ||
                        !writable ||
                        configuration?.provider === "disabled"
                      }
                    >
                      {t("chat.send")}
                    </button>
                  )}
                </div>
              </form>
            </section>
          )}
        </>
      )}
      {deleteOpen && (
        <div className="modal-backdrop">
          <section
            className="modal card"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="chat-delete-title"
          >
            <h2 id="chat-delete-title">{t("chat.confirmDelete")}</h2>
            <div className="actions">
              <button onClick={() => setDeleteOpen(false)}>
                {t("cancel")}
              </button>
              <button
                className="danger"
                disabled={busy}
                onClick={() =>
                  void act(async () => {
                    await api(conversationBase + "/" + selected, "DELETE", {});
                    setConversations((v) => v.filter((c) => c.id !== selected));
                    setSelected("");
                    setDeleteOpen(false);
                  })
                }
              >
                {t("delete")}
              </button>
            </div>
          </section>
        </div>
      )}
    </section>
  );
}
function AISettingsForm({
  settings,
  busy,
  onSave,
}: {
  settings: AISettings;
  busy: boolean;
  onSave: (s: AISettings) => void;
}) {
  const { t } = useTranslation();
  return (
    <section className="card chat-settings">
      <h2>{t("chat.settings")}</h2>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const v = Object.fromEntries(new FormData(e.currentTarget));
          onSave({
            ...v,
            offer_operator: v.offer_operator === "on",
          } as unknown as AISettings);
        }}
      >
        {(
          [
            "display_name",
            "role",
            "company_description",
            "style",
            "instructions",
            "no_answer",
          ] as const
        ).map((key) => (
          <label key={key}>
            {t("chat.fields." + key)}
            <textarea
              aria-label={t("chat.fields." + key)}
              name={key}
              defaultValue={settings[key]}
              maxLength={
                {
                  display_name: 120,
                  role: 200,
                  company_description: 2000,
                  style: 300,
                  instructions: 3000,
                  no_answer: 600,
                }[key]
              }
            />
          </label>
        ))}
        <label>
          {t("chat.languagePolicy")}
          <select
            name="language_policy"
            defaultValue={settings.language_policy}
          >
            {["auto", "fixed", "default"].map((v) => (
              <option key={v} value={v}>
                {t("chat.language." + v)}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t("chat.fixedLocale")}
          <select
            name="response_locale"
            defaultValue={settings.response_locale}
          >
            {locales.map((l) => (
              <option key={l} value={l}>
                {t("languages." + l)}
              </option>
            ))}
          </select>
        </label>
        <label className="checkbox-label">
          <input
            name="offer_operator"
            type="checkbox"
            defaultChecked={settings.offer_operator}
          />
          {t("chat.offerOperator")}
        </label>
        <button className="primary" disabled={busy}>
          {t("save")}
        </button>
      </form>
    </section>
  );
}
