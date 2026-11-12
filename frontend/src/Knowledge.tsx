import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, locales, type Assistant } from "./api";
type Source = {
  id: string;
  kind: string;
  title: string;
  content: string;
  language: string;
  url: string;
  format: string;
  status: string;
  error_code: string;
  enabled: boolean;
  revision: number;
  active_revision: number | null;
  imported_pages: number;
  created_at: string;
};
type Hit = {
  chunk_id: string;
  document_id: string;
  title: string;
  content: string;
  url: string;
  page: number | null;
  score: number;
  similarity: number;
  index_version: number;
  model: string;
  model_version: string;
  language: string;
};
export function Knowledge({
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
  const [tab, setTab] = useState("article");
  const [sources, setSources] = useState<Source[]>([]);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<Source | null>(null);
  const [form, setForm] = useState(false);
  const [preview, setPreview] = useState<Source | null>(null);
  const [deleting, setDeleting] = useState<Source | null>(null);
  const [hits, setHits] = useState<Hit[] | null>(null);
  const [query, setQuery] = useState("");
  const base =
    "/organizations/" +
    organizationId +
    "/assistants/" +
    assistant +
    "/knowledge";
  const message = (e: unknown) => (e instanceof ApiError ? e.code : "internal");
  async function refresh() {
    const v = await api<Source[]>(base);
    setSources(v);
  }
  useEffect(() => {
    if (!assistant) return;
    let active = true;
    setLoading(true);
    setSources([]);
    setForm(false);
    setPreview(null);
    setHits(null);
    setError("");
    const poll = async () => {
      try {
        const v = await api<Source[]>(base);
        if (active) setSources(v);
      } catch (e) {
        if (active) setError(message(e));
      } finally {
        if (active) setLoading(false);
      }
    };
    void poll();
    const timer = setInterval(() => void poll(), 3000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [base, assistant]);
  async function act(fn: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  const value = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    return Object.fromEntries(new FormData(e.currentTarget)) as Record<
      string,
      string
    >;
  };
  async function saveUpload(file: File) {
    const data = new FormData();
    data.append("file", file);
    let r: Response;
    try {
      r = await fetch("/api/v1" + base + "/upload", {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-CSRF-Protection": "1" },
        body: data,
      });
    } catch {
      throw new ApiError("network", 0);
    }
    const body = await r.json();
    if (!r.ok) throw new ApiError(body.error?.code || "internal", r.status);
    await refresh();
  }
  const locale = i18n.resolvedLanguage || "en";
  return (
    <section className="knowledge">
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
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      </label>
      {!assistant ? (
        <div className="empty card">{t("createAssistantFirst")}</div>
      ) : (
        <>
          <div
            className="knowledge-tabs"
            role="tablist"
            aria-label={t("knowledge")}
          >
            {["article", "document", "web", "search"].map((v) => (
              <button
                role="tab"
                aria-selected={tab === v}
                className={tab === v ? "active" : ""}
                key={v}
                onClick={() => {
                  setTab(v);
                  setForm(false);
                  setPreview(null);
                  setError("");
                }}
              >
                {t("kb.tabs." + v)}
              </button>
            ))}
          </div>
          {error && (
            <div className="alert" role="alert">
              {t("errors." + error, { defaultValue: t("errors.internal") })}
              <button aria-label={t("close")} onClick={() => setError("")}>
                ×
              </button>
            </div>
          )}
          {(loading || busy) && <p role="status">{t("loading")}</p>}
          {tab === "search" ? (
            <>
              <section className="card">
                <p className="info">{t("kb.searchNotice")}</p>
                <form
                  onSubmit={(e) => {
                    const v = value(e);
                    void act(async () => {
                      setHits(null);
                      setHits(
                        await api<Hit[]>(base + "/search", "POST", {
                          query: v.query,
                          mode: v.mode,
                        }),
                      );
                    });
                  }}
                >
                  <label>
                    {t("kb.question")}
                    <textarea
                      name="query"
                      value={query}
                      onChange={(e) => setQuery(e.target.value)}
                      required
                      maxLength={2000}
                    />
                  </label>
                  <label>
                    {t("kb.searchMode")}
                    <select name="mode">
                      <option value="hybrid">{t("kb.hybrid")}</option>
                      <option value="semantic">{t("kb.semantic")}</option>
                    </select>
                  </label>
                  <button className="primary" disabled={busy}>
                    {t("kb.search")}
                  </button>
                </form>
              </section>
              {hits !== null && (
                <div className="knowledge-results">
                  {hits.length === 0 ? (
                    <section className="empty card">
                      {t("kb.noResults")}
                    </section>
                  ) : (
                    hits.map((h) => (
                      <article className="card" key={h.chunk_id}>
                        <div className="page-heading">
                          <h2>{h.title}</h2>
                          <span className="badge">
                            {t("kb.score")}:{" "}
                            {new Intl.NumberFormat(locale, {
                              maximumFractionDigits: 4,
                            }).format(h.score)}
                          </span>
                        </div>
                        <p className="description">{h.content}</p>
                        {h.url && (
                          <a
                            href={h.url}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            {h.url}
                          </a>
                        )}
                        <p className="muted">
                          {h.page && (
                            <>
                              {t("kb.page")} {h.page} ·{" "}
                            </>
                          )}
                          {t("kb.version")} {h.index_version} · {h.model} ·{" "}
                          {t("kb.similarity")}{" "}
                          {new Intl.NumberFormat(locale, {
                            maximumFractionDigits: 3,
                          }).format(h.similarity)}
                        </p>
                      </article>
                    ))
                  )}
                </div>
              )}
            </>
          ) : (
            <>
              {writable && tab === "article" && !form && (
                <button
                  className="primary"
                  onClick={() => {
                    setEditing(null);
                    setForm(true);
                  }}
                >
                  {t("kb.createArticle")}
                </button>
              )}
              {writable && tab === "document" && (
                <section className="card">
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      const f = new FormData(e.currentTarget).get("file");
                      if (f instanceof File) void act(() => saveUpload(f));
                    }}
                  >
                    <label>
                      {t("kb.chooseFile")}
                      <input
                        type="file"
                        name="file"
                        accept=".txt,.md,.pdf,.docx"
                        required
                      />
                    </label>
                    <small>{t("kb.fileHint")}</small>
                    <button className="primary" disabled={busy}>
                      {t("kb.upload")}
                    </button>
                  </form>
                </section>
              )}
              {writable && tab === "web" && (
                <section className="card">
                  <form
                    onSubmit={(e) => {
                      const v = value(e);
                      void act(async () => {
                        await api(base + "/import-url", "POST", {
                          url: v.url,
                          crawl: v.crawl === "on",
                        });
                        await refresh();
                      });
                    }}
                  >
                    <label>
                      {t("kb.url")}
                      <input
                        type="url"
                        name="url"
                        required
                        maxLength={2000}
                        placeholder={t("kb.urlExample")}
                      />
                    </label>
                    <label className="checkbox-label">
                      <input type="checkbox" name="crawl" />
                      {t("kb.crawl")}
                    </label>
                    <p className="muted">{t("kb.crawlHint")}</p>
                    <button className="primary" disabled={busy}>
                      {t("kb.import")}
                    </button>
                  </form>
                </section>
              )}
              {form && (
                <section className="card article-editor">
                  <h2>{t(editing ? "kb.editArticle" : "kb.createArticle")}</h2>
                  <form
                    key={editing?.id || "new"}
                    onSubmit={(e) => {
                      const v = value(e);
                      void act(async () => {
                        await api(
                          base + (editing ? "/" + editing.id : "/articles"),
                          editing ? "PATCH" : "POST",
                          v,
                        );
                        setForm(false);
                        await refresh();
                      });
                    }}
                  >
                    <label>
                      {t("kb.title")}
                      <input
                        name="title"
                        defaultValue={editing?.title}
                        maxLength={240}
                        required
                      />
                    </label>
                    <label>
                      {t("kb.documentLanguage")}
                      <select
                        name="language"
                        defaultValue={editing?.language || ""}
                      >
                        <option value="">{t("kb.unknownLanguage")}</option>
                        {locales.map((l) => (
                          <option key={l} value={l}>
                            {t("languages." + l)}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label>
                      {t("kb.content")}
                      <textarea
                        aria-label={t("kb.content")}
                        name="content"
                        className="article-content"
                        defaultValue={editing?.content}
                        required
                        maxLength={1000000}
                      />
                    </label>
                    <div className="actions">
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => setForm(false)}
                      >
                        {t("cancel")}
                      </button>
                      <button className="primary" disabled={busy}>
                        {t("save")}
                      </button>
                    </div>
                  </form>
                </section>
              )}
              <div className="knowledge-list">
                {sources
                  .filter((s) => s.kind === tab)
                  .map((s) => (
                    <article className="card" key={s.id}>
                      <div className="knowledge-source-heading">
                        <div>
                          <h2>{s.title}</h2>
                          {s.url && (
                            <a
                              href={s.url}
                              target="_blank"
                              rel="noopener noreferrer"
                            >
                              {s.url}
                            </a>
                          )}
                        </div>
                        <span className="badge">
                          {t("kb.status." + s.status)}
                        </span>
                      </div>
                      <p className="muted">
                        {s.language
                          ? t("languages." + s.language)
                          : t("kb.unknownLanguage")}{" "}
                        ·{" "}
                        {new Intl.DateTimeFormat(locale, {
                          dateStyle: "medium",
                        }).format(new Date(s.created_at))}{" "}
                        · {t("kb.version")} {s.active_revision ?? "—"}
                      </p>
                      {s.imported_pages > 0 && (
                        <p>
                          {t("kb.pagesProcessed", { count: s.imported_pages })}
                        </p>
                      )}
                      {!s.enabled && <p className="info">{t("kb.disabled")}</p>}
                      {s.active_revision !== null &&
                        s.active_revision !== s.revision && (
                          <p className="info">{t("kb.oldIndex")}</p>
                        )}
                      {s.error_code && (
                        <p role="status" className="form-error">
                          {t("errors." + s.error_code, {
                            defaultValue: t("errors.processing_failed"),
                          })}
                        </p>
                      )}
                      <div className="knowledge-actions">
                        <button
                          disabled={busy}
                          onClick={() =>
                            void act(async () =>
                              setPreview(await api<Source>(base + "/" + s.id)),
                            )
                          }
                        >
                          {t("kb.view")}
                        </button>
                        {writable && (
                          <>
                            {s.kind === "article" && (
                              <button
                                disabled={busy}
                                onClick={() =>
                                  void act(async () => {
                                    setEditing(
                                      await api<Source>(base + "/" + s.id),
                                    );
                                    setForm(true);
                                  })
                                }
                              >
                                {t("edit")}
                              </button>
                            )}
                            <button
                              disabled={busy}
                              onClick={() =>
                                void act(async () => {
                                  await api(base + "/" + s.id, "PATCH", {
                                    enabled: !s.enabled,
                                  });
                                  await refresh();
                                })
                              }
                            >
                              {t(s.enabled ? "kb.disable" : "kb.enable")}
                            </button>
                            <button
                              disabled={
                                busy ||
                                s.status === "processing" ||
                                s.status === "pending"
                              }
                              onClick={() =>
                                void act(async () => {
                                  await api(
                                    base + "/" + s.id + "/reindex",
                                    "POST",
                                    {},
                                  );
                                  await refresh();
                                })
                              }
                            >
                              {t(
                                s.kind === "web" ? "kb.refresh" : "kb.reindex",
                              )}
                            </button>
                            <button
                              className="danger"
                              disabled={busy}
                              onClick={() => setDeleting(s)}
                            >
                              {t("delete")}
                            </button>
                          </>
                        )}
                      </div>
                    </article>
                  ))}
              </div>
              {!loading && !sources.some((s) => s.kind === tab) && (
                <section className="empty card">
                  <h2>{t("kb.empty")}</h2>
                  <p>{t("kb.emptyHint")}</p>
                </section>
              )}
              {preview && (
                <section className="card knowledge-preview">
                  <div className="modal-heading">
                    <h2>{preview.title}</h2>
                    <button
                      onClick={() => setPreview(null)}
                      aria-label={t("close")}
                    >
                      ×
                    </button>
                  </div>
                  <p className="description">
                    {preview.content || t("kb.notExtracted")}
                  </p>
                </section>
              )}
            </>
          )}
        </>
      )}
      {deleting && (
        <div className="modal-backdrop">
          <section
            className="card modal"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="knowledge-delete-title"
          >
            <h2 id="knowledge-delete-title">{t("kb.confirmDelete")}</h2>
            <p>{deleting.title}</p>
            <div className="actions">
              <button disabled={busy} onClick={() => setDeleting(null)}>
                {t("cancel")}
              </button>
              <button
                className="danger"
                disabled={busy}
                onClick={() =>
                  void act(async () => {
                    await api(base + "/" + deleting.id, "DELETE", {});
                    setDeleting(null);
                    setHits(null);
                    await refresh();
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
