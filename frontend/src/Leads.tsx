import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, type Assistant, type Site } from "./api";
import type { Lead, LeadDetail } from "./leads-api";
import { LeadFields, useFieldLabel } from "./LeadFields";
export function Leads({
  organizationId,
  userId,
  role,
  sites,
  assistants,
  initialLead = "",
  onConversation,
}: {
  organizationId: string;
  userId: string;
  role: string;
  sites: Site[];
  assistants: Assistant[];
  initialLead?: string;
  onConversation: (id: string) => void;
}) {
  const { t, i18n } = useTranslation(),
    tr = (k: string) => t("lead." + k),
    label = useFieldLabel();
  const [items, setItems] = useState<Lead[]>([]),
    [next, setNext] = useState(""),
    [cursor, setCursor] = useState(""),
    [selected, setSelected] = useState(initialLead),
    [detail, setDetail] = useState<LeadDetail | null>(null),
    [members, setMembers] = useState<{ id: string; name: string }[]>([]),
    [filter, setFilter] = useState({
      status: "",
      site: "",
      assistant: "",
      assigned: "",
      from: "",
      to: "",
    }),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true),
    [edit, setEdit] = useState(false),
    [values, setValues] = useState<Record<string, string | number | null>>({}),
    [note, setNote] = useState(""),
    [confirmDelete, setConfirmDelete] = useState(false);
  const base = `/organizations/${organizationId}/leads`;
  const fmt = (s: string) =>
    new Intl.DateTimeFormat(i18n.resolvedLanguage, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(s));
  function fail(e: unknown) {
    setError(e instanceof ApiError ? e.code : "internal");
  }
  async function load() {
    const p = new URLSearchParams({ ...filter, cursor });
    const d = await api<{ items: Lead[]; next_cursor: string }>(base + "?" + p);
    setItems(d.items);
    setNext(d.next_cursor);
  }
  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    const p = new URLSearchParams({ ...filter, cursor });
    void api<{ items: Lead[]; next_cursor: string }>(base + "?" + p)
      .then((d) => {
        if (active) {
          setItems(d.items);
          setNext(d.next_cursor);
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
    };
  }, [base, JSON.stringify(filter), cursor]);
  useEffect(() => {
    let active = true;
    setDetail(null);
    setEdit(false);
    setConfirmDelete(false);
    setNote("");
    if (selected)
      void api<LeadDetail>(base + "/" + selected)
        .then((d) => {
          if (active) setDetail(d);
        })
        .catch((e) => {
          if (active) fail(e);
        });
    return () => {
      active = false;
    };
  }, [base, selected]);
  useEffect(() => {
    void api<{ id: string; name: string }[]>(
      `/organizations/${organizationId}/members`,
    )
      .then(setMembers)
      .catch(fail);
  }, [organizationId]);
  async function patch(body: Record<string, unknown>) {
    if (!detail) return;
    setBusy(true);
    setError("");
    try {
      setDetail(
        await api<LeadDetail>(base + "/" + selected, "PATCH", {
          version: detail.lead.version,
          ...body,
        }),
      );
      setEdit(false);
      setNote("");
      await load();
    } catch (e) {
      fail(e);
      setDetail(
        await api<LeadDetail>(base + "/" + selected).catch(() => detail),
      );
    } finally {
      setBusy(false);
    }
  }
  const l = detail?.lead,
    own = l?.assigned_user_id === userId,
    canWrite = role !== "operator" || own,
    member = (id: string | null) =>
      members.find((m) => m.id === id)?.name ||
      (id ? tr("staff") : tr("unassigned"));
  return (
    <section className={"leads-page " + (selected ? "lead-selected" : "")}>
      <div className="lead-filters">
        {(["status", "site", "assistant", "assigned"] as const).map((k) => (
          <label key={k}>
            {tr(k)}
            <select
              value={filter[k]}
              onChange={(e) => {
                setCursor("");
                setFilter({ ...filter, [k]: e.target.value });
              }}
            >
              <option value="">{tr("all")}</option>
              {k === "status"
                ? ["new", "in_progress", "won", "lost"].map((v) => (
                    <option key={v} value={v}>
                      {tr("status_" + v)}
                    </option>
                  ))
                : (k === "site"
                    ? sites
                    : k === "assistant"
                      ? assistants
                      : members
                  ).map((v) => (
                    <option key={v.id} value={v.id}>
                      {v.name || tr("staff")}
                    </option>
                  ))}
            </select>
          </label>
        ))}
        {(["from", "to"] as const).map((k) => (
          <label key={k}>
            {tr(k)}
            <input
              type="date"
              value={filter[k]}
              onChange={(e) => {
                setCursor("");
                setFilter({ ...filter, [k]: e.target.value });
              }}
            />
          </label>
        ))}
      </div>
      {error && (
        <p className="alert" role="alert">
          {t("errors." + error, { defaultValue: t("errors.internal") })}
        </p>
      )}
      <div className="lead-workspace">
        <section className="lead-list card" aria-label={tr("list")}>
          {loading && <p role="status">{t("loading")}</p>}
          {!loading && items.length === 0 && <p>{tr("empty")}</p>}
          {items.map((v) => (
            <button
              className={selected === v.id ? "lead-row selected" : "lead-row"}
              key={v.id}
              onClick={() => setSelected(v.id)}
            >
              <strong>{v.name || tr("unnamed")}</strong>
              <span>{v.email || v.phone}</span>
              <span>{v.site}</span>
              <span className="lead-summary">
                {String(v.values.request_description?.value || "")}
              </span>
              <small>
                {tr("status_" + v.status)} · {member(v.assigned_user_id)}
              </small>
              <time>{fmt(v.created_at)}</time>
            </button>
          ))}
          <div className="lead-actions">
            {cursor && (
              <button onClick={() => setCursor("")}>{tr("first")}</button>
            )}
            {next && (
              <button onClick={() => setCursor(next)}>{tr("more")}</button>
            )}
            <button onClick={() => void load().catch(fail)}>
              {tr("refresh")}
            </button>
          </div>
        </section>
        <section className="card lead-detail" aria-label={tr("card")}>
          {selected && (
            <button className="lead-back" onClick={() => setSelected("")}>
              {tr("back")}
            </button>
          )}
          {!selected ? (
            <p>{tr("choose")}</p>
          ) : !l ? (
            <p role="status">{t("loading")}</p>
          ) : (
            <>
              <div className="page-heading">
                <div>
                  <h2>{l.name || tr("unnamed")}</h2>
                  <p>
                    {l.site} · {fmt(l.created_at)}
                  </p>
                </div>
                <span className="lead-badge">{tr("status_" + l.status)}</span>
              </div>
              {l.can_open_conversation && (
                <button onClick={() => onConversation(l.conversation_id)}>
                  {tr("conversation")}
                </button>
              )}
              <p className="muted">
                {tr("source")}: {tr("source_" + l.source)}
              </p>
              {!l.assigned_user_id && role === "operator" && (
                <button
                  className="primary"
                  disabled={busy}
                  onClick={() => void patch({ assigned_user_id: userId })}
                >
                  {tr("claim")}
                </button>
              )}
              <div className="lead-form-grid">
                <label>
                  {tr("status")}
                  <select
                    aria-label={tr("status")}
                    disabled={!canWrite || busy}
                    value={l.status}
                    onChange={(e) => void patch({ status: e.target.value })}
                  >
                    {["new", "in_progress", "won", "lost"].map((v) => (
                      <option key={v} value={v}>
                        {tr("status_" + v)}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {tr("assigned")}
                  <select
                    aria-label={tr("assigned")}
                    disabled={role === "operator" || busy}
                    value={l.assigned_user_id || ""}
                    onChange={(e) =>
                      void patch({ assigned_user_id: e.target.value })
                    }
                  >
                    <option value="">{tr("unassigned")}</option>
                    {members.map((m) => (
                      <option key={m.id} value={m.id}>
                        {member(m.id)}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              {edit ? (
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    void patch({ values });
                  }}
                >
                  <LeadFields
                    fields={l.snapshot.fields}
                    values={values}
                    onChange={(k, v) =>
                      setValues((old) => ({ ...old, [k]: v }))
                    }
                  />
                  <button className="primary" disabled={busy}>
                    {tr("saveData")}
                  </button>
                  <button type="button" onClick={() => setEdit(false)}>
                    {tr("back")}
                  </button>
                </form>
              ) : (
                <>
                  <dl className="lead-data">
                    {l.snapshot.fields
                      .filter((f) => l.values[f.key])
                      .map((f) => (
                        <div key={f.key}>
                          <dt>{label(f)}</dt>
                          <dd>
                            {String(l.values[f.key].value)}
                            <small>
                              {tr("provenance_" + l.values[f.key].source)} ·{" "}
                              {fmt(l.values[f.key].updated_at)}
                            </small>
                          </dd>
                        </div>
                      ))}
                  </dl>
                  {role !== "operator" && (
                    <button
                      disabled={busy}
                      onClick={() => {
                        setValues(
                          Object.fromEntries(
                            Object.entries(l.values).map(([k, v]) => [
                              k,
                              v.value,
                            ]),
                          ),
                        );
                        setEdit(true);
                      }}
                    >
                      {tr("editData")}
                    </button>
                  )}
                </>
              )}
              {l.privacy_url && (
                <a
                  href={l.privacy_url}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {tr("privacy")}
                </a>
              )}
              <h3>{tr("history")}</h3>
              <div className="lead-history">
                {detail.events.map((e) => (
                  <article key={e.id}>
                    <strong>
                      {tr("event_" + e.kind)}
                      {e.status ? " · " + tr("status_" + e.status) : ""}
                    </strong>
                    {e.note && <p>{e.note}</p>}
                    <small>
                      {member(e.actor_id)}
                      {e.target_id ? " → " + member(e.target_id) : ""} ·{" "}
                      {fmt(e.created_at)}
                    </small>
                  </article>
                ))}
              </div>
              {canWrite && (
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    void patch({ note });
                  }}
                >
                  <label>
                    {tr("note")}
                    <textarea
                      disabled={busy}
                      maxLength={4000}
                      value={note}
                      onChange={(e) => setNote(e.target.value)}
                      required
                    />
                  </label>
                  <button disabled={busy || !note.trim()}>
                    {tr("addNote")}
                  </button>
                </form>
              )}
              {role !== "operator" && (
                <button
                  className="danger"
                  disabled={busy}
                  onClick={() => setConfirmDelete(true)}
                >
                  {tr("delete")}
                </button>
              )}
              {confirmDelete && (
                <section
                  className="lead-delete"
                  role="alertdialog"
                  aria-label={tr("delete")}
                >
                  <p>{tr("deleteConfirm")}</p>
                  <button autoFocus onClick={() => setConfirmDelete(false)}>
                    {tr("back")}
                  </button>
                  <button
                    className="danger"
                    disabled={busy}
                    onClick={() => {
                      setBusy(true);
                      void api(base + "/" + selected, "DELETE")
                        .then(async () => {
                          setSelected("");
                          setDetail(null);
                          await load();
                        })
                        .catch(fail)
                        .finally(() => setBusy(false));
                    }}
                  >
                    {tr("delete")}
                  </button>
                </section>
              )}
            </>
          )}
        </section>
      </div>
    </section>
  );
}
