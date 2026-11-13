import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, type Site, type Assistant } from "./api";
type Conversation = {
  id: string;
  site: string;
  assistant: string;
  control_mode: string;
  state_version: number;
  assigned_user_id: string | null;
  assignee: string;
  unread: number;
  last_content: string;
  created_at: string;
  last_message_at: string | null;
  generating: boolean;
};
type Message = {
  id: string;
  sequence: number;
  change_seq: number;
  role: string;
  content: string;
  status: string;
  created_at: string;
  author_user_id: string | null;
};
type Event = {
  id: string;
  sequence: number;
  kind: string;
  content: string;
  actor_id: string | null;
  target_id: string | null;
  created_at: string;
};
type Member = { id: string; name: string; role: string };
type Page = {
  conversation: Conversation;
  messages: Message[];
  events: Event[];
  cursor: number;
  next_before: number;
  more: boolean;
};
const merge = <T extends { id: string; change_seq?: number }>(
  a: T[],
  b: T[],
) => {
  const map = new Map(a.map((v) => [v.id, v]));
  for (const v of b) {
    if ((map.get(v.id)?.change_seq || 0) > (v.change_seq || 0)) continue;
    map.set(v.id, v);
  }
  return Array.from(map.values());
};
export function Inbox({
  organizationId,
  userId,
  role,
  sites,
  assistants,
}: {
  organizationId: string;
  userId: string;
  role: string;
  sites: Site[];
  assistants: Assistant[];
}) {
  const { t, i18n } = useTranslation();
  const tr = (k: string) => t("inbox." + k);
  const base = "/organizations/" + organizationId + "/conversations";
  const [items, setItems] = useState<Conversation[]>([]),
    [next, setNext] = useState(""),
    [cursor, setCursor] = useState(""),
    [filter, setFilter] = useState({
      mode: "",
      site: "",
      assistant: "",
      assigned: "",
      q: "",
      from: "",
      to: "",
    });
  const [selected, setSelected] = useState(""),
    [data, setData] = useState<Page | null>(null),
    [members, setMembers] = useState<Member[]>([]),
    [text, setText] = useState(""),
    [note, setNote] = useState(""),
    [target, setTarget] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true),
    [confirmClose, setConfirmClose] = useState(false);
  const change = useRef(0),
    before = useRef(0),
    lastRead = useRef(0),
    seenRead = useRef(0),
    detailEpoch = useRef(0);
  const replyRetry = useRef<{
    key: string;
    body: Record<string, unknown>;
  } | null>(null);
  const historyElement = useRef<HTMLDivElement | null>(null),
    stick = useRef(true),
    closeDialog = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (stick.current && historyElement.current)
      historyElement.current.scrollTop = historyElement.current.scrollHeight;
  }, [data?.messages.length, data?.events.length]);
  useEffect(() => {
    if (!confirmClose) return;
    const el = closeDialog.current;
    const buttons = el?.querySelectorAll<HTMLButtonElement>("button");
    buttons?.[0]?.focus();
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setConfirmClose(false);
      }
      if (e.key === "Tab" && buttons?.length) {
        e.preventDefault();
        (document.activeElement === buttons[0]
          ? buttons[1]
          : buttons[0]
        ).focus();
      }
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [confirmClose]);
  const fmt = (s: string) =>
    new Intl.DateTimeFormat(i18n.resolvedLanguage, {
      dateStyle: "short",
      timeStyle: "short",
    }).format(new Date(s));
  function problem(e: unknown) {
    setError(e instanceof ApiError ? e.code : "network");
  }
  function acceptPage(result: Page) {
    seenRead.current = Math.max(
      seenRead.current,
      0,
      ...result.messages
        .filter((m) => m.role === "user")
        .map((m) => m.sequence),
    );
    if (!change.current) before.current = result.more ? result.next_before : 0;
    change.current = Math.max(change.current, result.cursor);
    setData((old) => ({
      ...result,
      conversation:
        old &&
        old.conversation.state_version > result.conversation.state_version
          ? old.conversation
          : result.conversation,
      messages: merge(old?.messages || [], result.messages).sort(
        (a, b) => a.sequence - b.sequence,
      ),
      events: merge(old?.events || [], result.events).sort(
        (a, b) => a.sequence - b.sequence,
      ),
    }));
  }
  async function acknowledge(epoch: number) {
    const read = seenRead.current;
    if (read <= lastRead.current || document.hidden) return;
    await api(base + "/" + selected + "/read", "POST", { sequence: read });
    if (epoch === detailEpoch.current)
      lastRead.current = Math.max(lastRead.current, read);
  }
  useEffect(() => {
    void api<Member[]>("/organizations/" + organizationId + "/members")
      .then(setMembers)
      .catch(problem);
  }, [organizationId]);
  useEffect(() => {
    let active = true,
      running = false;
    setLoading(true);
    const load = async () => {
      if (running || document.hidden) return;
      running = true;
      try {
        const query = new URLSearchParams({ ...filter, cursor });
        const result = await api<{
          items: Conversation[];
          next_cursor: string;
        }>(base + "?" + query);
        if (active) {
          setItems(result.items);
          setNext(result.next_cursor);
          setLoading(false);
        }
      } catch (e) {
        if (active) {
          problem(e);
          setLoading(false);
        }
      } finally {
        running = false;
      }
    };
    const delay = setTimeout(() => void load(), 250);
    const timer = setInterval(() => void load(), 3000);
    const visible = () => void load();
    document.addEventListener("visibilitychange", visible);
    return () => {
      active = false;
      clearTimeout(delay);
      clearInterval(timer);
      document.removeEventListener("visibilitychange", visible);
    };
  }, [base, filter, cursor]);
  useEffect(() => {
    setData(null);
    setText("");
    setNote("");
    setError("");
    change.current = 0;
    before.current = 0;
    lastRead.current = 0;
    seenRead.current = 0;
    replyRetry.current = null;
    stick.current = true;
    const epoch = ++detailEpoch.current;
    if (!selected) return;
    let active = true,
      running = false;
    const load = async () => {
      if (running || document.hidden) return;
      running = true;
      try {
        const result = await api<Page>(
          base +
            "/" +
            selected +
            "/messages" +
            (change.current ? "?after=" + change.current : ""),
        );
        if (!active || epoch !== detailEpoch.current) return;
        acceptPage(result);
        await acknowledge(epoch);
      } catch (e) {
        if (active) {
          problem(e);
          if (e instanceof ApiError && (e.status === 403 || e.status === 404)) {
            setData(null);
            setSelected("");
          }
        }
      } finally {
        running = false;
      }
    };
    void load();
    const timer = setInterval(() => void load(), 3000);
    const visible = () => void load();
    document.addEventListener("visibilitychange", visible);
    return () => {
      active = false;
      clearInterval(timer);
      document.removeEventListener("visibilitychange", visible);
    };
  }, [base, selected]);
  const c = data?.conversation;
  const writable =
    c?.control_mode === "human_active" &&
    (role !== "operator" || c.assigned_user_id === userId);
  const own = c?.assigned_user_id === userId;
  const member = (id: string | null) =>
    members.find((m) => m.id === id)?.name ||
    tr("staff") + (id ? " #" + id.slice(0, 6) : "");
  async function act(kind: string) {
    if (!c || busy) return;
    const epoch = detailEpoch.current;
    setBusy(true);
    setError("");
    try {
      const body: {
        request_id: string;
        version: number;
        content?: string;
        assignee?: string;
      } = {
        request_id: crypto.randomUUID(),
        version: c.state_version,
        ...(kind === "messages"
          ? { content: text }
          : kind === "notes"
            ? { content: note }
            : kind === "assign"
              ? { assignee: target }
              : {}),
      };
      const retryKey = JSON.stringify([
        selected,
        kind,
        c.state_version,
        body.content,
        body.assignee,
      ]);
      if (replyRetry.current?.key === retryKey)
        body.request_id = replyRetry.current.body.request_id as string;
      replyRetry.current = { key: retryKey, body };
      const result = await api<Conversation | Message>(
        base + "/" + selected + "/" + kind,
        "POST",
        body,
      );
      if (epoch !== detailEpoch.current) return;
      replyRetry.current = null;
      if (kind === "messages") {
        setText("");
        setData((old) =>
          old
            ? {
                ...old,
                messages: merge(old.messages, [result as Message]).sort(
                  (a, b) => a.sequence - b.sequence,
                ),
              }
            : old,
        );
      } else if (kind === "notes") {
        setNote("");
      } else
        setData((old) =>
          old ? { ...old, conversation: result as Conversation } : old,
        );
      setConfirmClose(false);
      try {
        const fresh = await api<Page>(
          base +
            "/" +
            selected +
            "/messages" +
            (change.current ? "?after=" + change.current : ""),
        );
        if (epoch === detailEpoch.current) {
          acceptPage(fresh);
          await acknowledge(epoch);
        }
      } catch (refreshError) {
        if (epoch === detailEpoch.current) problem(refreshError);
      }
    } catch (e) {
      if (epoch !== detailEpoch.current) return;
      problem(e);
      if (e instanceof ApiError && e.code === "state_conflict") {
        try {
          const fresh = await api<Page>(base + "/" + selected);
          if (epoch === detailEpoch.current) {
            setData(fresh);
            change.current = fresh.cursor;
          }
        } catch (refreshError) {
          problem(refreshError);
          setData(null);
          setSelected("");
        }
      }
    } finally {
      setBusy(false);
    }
  }
  async function older() {
    if (!before.current || busy) return;
    setBusy(true);
    stick.current = false;
    try {
      const p = await api<Page>(
        base + "/" + selected + "/messages?before=" + before.current,
      );
      before.current = p.more ? p.next_before : 0;
      setData((old) =>
        old
          ? {
              ...old,
              messages: merge(p.messages, old.messages).sort(
                (a, b) => a.sequence - b.sequence,
              ),
              events: merge(p.events, old.events).sort(
                (a, b) => a.sequence - b.sequence,
              ),
            }
          : p,
      );
    } catch (e) {
      problem(e);
    } finally {
      setBusy(false);
    }
  }
  const timeline = data
    ? [
        ...data.messages.map((m) => ({
          id: m.id,
          date: m.created_at,
          message: m,
          event: null as Event | null,
        })),
        ...data.events
          .filter((e) => e.kind !== "note")
          .map((e) => ({
            id: e.id,
            date: e.created_at,
            message: null as Message | null,
            event: e,
          })),
      ].sort(
        (a, b) =>
          new Date(a.date).getTime() - new Date(b.date).getTime() ||
          (a.message && b.message
            ? a.message.sequence - b.message.sequence
            : 0) ||
          a.id.localeCompare(b.id),
      )
    : [];
  return (
    <section className={"inbox-shell " + (selected ? "inbox-open" : "")}>
      <div className="inbox-filters">
        <label>
          {tr("filter")}
          <select
            aria-label={tr("filter")}
            value={filter.mode}
            onChange={(e) => {
              setFilter({ ...filter, mode: e.target.value });
              setCursor("");
            }}
          >
            {["", "human_requested", "ai_active", "human_active", "closed"].map(
              (v) => (
                <option key={v} value={v}>
                  {tr(v || "all")}
                </option>
              ),
            )}
          </select>
        </label>
        <label>
          {tr("search")}
          <input
            type="search"
            maxLength={200}
            value={filter.q}
            onChange={(e) => {
              setFilter({ ...filter, q: e.target.value });
              setCursor("");
            }}
          />
        </label>
        <label>
          {tr("site")}
          <select
            aria-label={tr("site")}
            value={filter.site}
            onChange={(e) => {
              setFilter({ ...filter, site: e.target.value });
              setCursor("");
            }}
          >
            <option value="">{tr("all")}</option>
            {sites.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          {tr("assistant")}
          <select
            aria-label={tr("assistant")}
            value={filter.assistant}
            onChange={(e) => {
              setFilter({ ...filter, assistant: e.target.value });
              setCursor("");
            }}
          >
            <option value="">{tr("all")}</option>
            {assistants.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          {tr("assigned")}
          <select
            aria-label={tr("assigned")}
            value={filter.assigned}
            onChange={(e) => {
              setFilter({ ...filter, assigned: e.target.value });
              setCursor("");
            }}
          >
            <option value="">{tr("all")}</option>
            <option value="me">{tr("mine")}</option>
            {members.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name || tr("staff") + " #" + m.id.slice(0, 6)}
              </option>
            ))}
          </select>
        </label>
        <label>
          {tr("from")}
          <input
            type="date"
            value={filter.from}
            onChange={(e) => {
              setFilter({ ...filter, from: e.target.value });
              setCursor("");
            }}
          />
        </label>
        <label>
          {tr("to")}
          <input
            type="date"
            value={filter.to}
            onChange={(e) => {
              setFilter({ ...filter, to: e.target.value });
              setCursor("");
            }}
          />
        </label>
      </div>
      {error && (
        <div className="alert" role="alert">
          {t("errors." + error, { defaultValue: t("errors.internal") })}
        </div>
      )}
      <div className="inbox-columns">
        <section className="inbox-list" aria-label={tr("title")}>
          {loading ? (
            <p role="status">{tr("loading")}</p>
          ) : !items.length ? (
            <p className="empty-small">{tr("empty")}</p>
          ) : (
            items.map((v) => (
              <button
                key={v.id}
                className={
                  "conversation-row " + (selected === v.id ? "selected" : "")
                }
                onClick={() => setSelected(v.id)}
              >
                <div>
                  <strong>
                    {tr("visitor")} #{v.id.slice(0, 6)}
                  </strong>
                  {v.unread > 0 && (
                    <span className="inbox-count">{v.unread}</span>
                  )}
                </div>
                <small>
                  {v.site} · {fmt(v.last_message_at || v.created_at)}
                </small>
                <p>{v.last_content || tr("emptyConversation")}</p>
                <div>
                  <span className={"mode-chip " + v.control_mode}>
                    {tr(v.control_mode)}
                  </span>
                  {v.assigned_user_id && (
                    <small>{member(v.assigned_user_id)}</small>
                  )}
                </div>
              </button>
            ))
          )}
          <div className="inbox-pagination">
            {cursor && (
              <button onClick={() => setCursor("")}>{tr("first")}</button>
            )}
            {next && (
              <button onClick={() => setCursor(next)}>{tr("next")}</button>
            )}
          </div>
        </section>
        <section className="inbox-thread" aria-label={tr("history")}>
          {!c ? (
            <div className="empty">
              <h2>{tr("select")}</h2>
              <p>{tr("selectHint")}</p>
            </div>
          ) : (
            <>
              <header className="thread-header">
                <button className="inbox-back" onClick={() => setSelected("")}>
                  {tr("back")}
                </button>
                <div>
                  <h2>
                    {tr("visitor")} #{c.id.slice(0, 6)}
                  </h2>
                  <p>
                    {c.site} · {tr(c.control_mode)}
                  </p>
                </div>
                <div className="thread-actions">
                  {c.control_mode !== "closed" && (
                    <>
                      {(!c.assigned_user_id || own) && (
                        <button
                          disabled={busy}
                          className="primary"
                          onClick={() => void act("take")}
                        >
                          {tr("take")}
                        </button>
                      )}
                      {(own || role !== "operator") && (
                        <>
                          {c.control_mode !== "ai_active" && (
                            <button
                              disabled={busy}
                              onClick={() => void act("resume-ai")}
                            >
                              {tr("resume")}
                            </button>
                          )}
                          <button
                            disabled={busy}
                            onClick={() => setConfirmClose(true)}
                          >
                            {tr("close")}
                          </button>
                        </>
                      )}
                    </>
                  )}
                </div>
              </header>
              <div
                className="inbox-history"
                ref={historyElement}
                onScroll={(e) => {
                  const el = e.currentTarget;
                  stick.current =
                    el.scrollHeight - el.scrollTop - el.clientHeight < 70;
                }}
              >
                {before.current > 0 && (
                  <button
                    className="history-more"
                    disabled={busy}
                    onClick={() => void older()}
                  >
                    {tr("older")}
                  </button>
                )}
                {timeline.map((row) =>
                  row.message ? (
                    <article
                      key={row.id}
                      className={"inbox-message " + row.message.role}
                    >
                      <strong>
                        {row.message.role === "user"
                          ? tr("visitor")
                          : row.message.role === "operator"
                            ? member(row.message.author_user_id)
                            : tr("virtual")}
                      </strong>
                      <p>
                        {row.message.content ||
                          tr(
                            row.message.status === "processing"
                              ? "generating"
                              : row.message.status === "canceled"
                                ? "canceled"
                                : "failed",
                          )}
                      </p>
                      <time>{fmt(row.date)}</time>
                    </article>
                  ) : (
                    <p className="inbox-event" key={row.id}>
                      {tr("event_" + row.event!.kind)} ·{" "}
                      {member(row.event!.actor_id)} · {fmt(row.date)}
                    </p>
                  ),
                )}
              </div>
              <form
                className="operator-composer"
                onSubmit={(e) => {
                  e.preventDefault();
                  void act("messages");
                }}
              >
                <label>
                  {tr("reply")}
                  <textarea
                    value={text}
                    maxLength={2000}
                    disabled={!writable || busy}
                    placeholder={tr(
                      c.control_mode === "closed"
                        ? "closedHint"
                        : writable
                          ? "replyPlaceholder"
                          : "takeFirst",
                    )}
                    onChange={(e) => setText(e.target.value)}
                    onKeyDown={(e) => {
                      if (
                        e.key === "Enter" &&
                        !e.shiftKey &&
                        !e.nativeEvent.isComposing &&
                        text.trim()
                      ) {
                        e.preventDefault();
                        void act("messages");
                      }
                    }}
                  />
                </label>
                <button
                  className="primary"
                  disabled={!writable || busy || !text.trim()}
                >
                  {tr("send")}
                </button>
              </form>
            </>
          )}
        </section>
        {c && (
          <aside className="inbox-info">
            <h3>{tr("details")}</h3>
            <dl>
              <dt>{tr("site")}</dt>
              <dd>{c.site}</dd>
              <dt>{tr("assistant")}</dt>
              <dd>{c.assistant}</dd>
              <dt>{tr("started")}</dt>
              <dd>{fmt(c.created_at)}</dd>
              <dt>{tr("mode")}</dt>
              <dd>{tr(c.control_mode)}</dd>
              <dt>{tr("identifier")}</dt>
              <dd className="technical-id">{c.id}</dd>
              <dt>{tr("assigned")}</dt>
              <dd>
                {c.assigned_user_id
                  ? member(c.assigned_user_id)
                  : tr("unassigned")}
              </dd>
            </dl>
            {role !== "operator" && c.control_mode !== "closed" && (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void act("assign");
                }}
              >
                <label>
                  {tr("assign")}
                  <select
                    aria-label={tr("assign")}
                    value={target}
                    onChange={(e) => setTarget(e.target.value)}
                  >
                    <option value="">{tr("unassigned")}</option>
                    {members.map((m) => (
                      <option value={m.id} key={m.id}>
                        {m.name || tr("staff") + " #" + m.id.slice(0, 6)}
                      </option>
                    ))}
                  </select>
                </label>
                <button disabled={busy}>{tr("assign")}</button>
              </form>
            )}
            <h3>{tr("notes")}</h3>
            <p className="muted">{tr("private")}</p>
            {data?.events
              .filter((e) => e.kind === "note")
              .map((e) => (
                <article className="internal-note" key={e.id}>
                  <p>{e.content}</p>
                  <small>
                    {member(e.actor_id)} · {fmt(e.created_at)}
                  </small>
                </article>
              ))}
            {c.control_mode !== "closed" && (role !== "operator" || own) && (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  void act("notes");
                }}
              >
                <label>
                  {tr("addNote")}
                  <textarea
                    value={note}
                    maxLength={4000}
                    onChange={(e) => setNote(e.target.value)}
                  />
                </label>
                <button disabled={busy || !note.trim()}>{tr("addNote")}</button>
              </form>
            )}
            <h3>{tr("transfers")}</h3>
            {data?.events
              .filter((e) => e.kind !== "note")
              .map((e) => (
                <p className="transfer-row" key={e.id}>
                  {tr("event_" + e.kind)}
                  {e.target_id ? " · " + member(e.target_id) : ""}
                  <small>{fmt(e.created_at)}</small>
                </p>
              ))}
          </aside>
        )}
      </div>
      {confirmClose && (
        <div className="modal-backdrop">
          <section
            ref={closeDialog}
            className="modal"
            role="alertdialog"
            aria-modal="true"
            aria-label={tr("close")}
          >
            <h2>{tr("closeConfirm")}</h2>
            <p>{tr("closedHint")}</p>
            <button autoFocus onClick={() => setConfirmClose(false)}>
              {tr("back")}
            </button>
            <button
              className="primary"
              disabled={busy}
              onClick={() => void act("close")}
            >
              {tr("close")}
            </button>
          </section>
        </div>
      )}
    </section>
  );
}
