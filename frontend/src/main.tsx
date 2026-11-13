import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { useTranslation } from "react-i18next";
import i18n from "./i18n";
import {
  api,
  ApiError,
  locales,
  type Locale,
  type User,
  type Organization,
  type Assistant,
  type Site,
} from "./api";
import "./style.css";
import { Knowledge } from "./Knowledge";
import { PreviewChat } from "./PreviewChat";
import { WidgetSettings } from "./WidgetSettings";

function App() {
  const { t } = useTranslation();
  const [user, setUser] = useState<User | null>(null);
  const [boot, setBoot] = useState(true);
  const [brand, setBrand] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [register, setRegister] = useState(false);
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [org, setOrg] = useState<Organization | null>(null);
  const [assistants, setAssistants] = useState<Assistant[]>([]);
  const [knowledgeAssistant, setKnowledgeAssistant] = useState("");
  const [widgetSite, setWidgetSite] = useState("");
  const [sites, setSites] = useState<Site[]>([]);
  const [page, setPage] = useState("home");
  const [dialog, setDialog] = useState("");
  const [editing, setEditing] = useState<Assistant | null>(null);
  const [siteDelete, setSiteDelete] = useState<Site | null>(null);
  useEffect(() => {
    if (!dialog && !siteDelete) return;
    const previous = document.activeElement as HTMLElement | null;
    const modal = document.querySelector<HTMLElement>(
      '[role="dialog"], [role="alertdialog"]',
    );
    if (!modal) return;
    const controls = () =>
      Array.from(
        modal.querySelectorAll<HTMLElement>(
          "button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled)",
        ),
      );
    controls()[0]?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) {
        setDialog("");
        setSiteDelete(null);
      }
      if (event.key === "Tab") {
        const items = controls();
        const first = items[0];
        const last = items.at(-1);
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last?.focus();
        }
        if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first?.focus();
        }
      }
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      previous?.focus();
    };
  }, [dialog, siteDelete, busy]);
  const language = i18n.resolvedLanguage || "en";
  useEffect(() => {
    document.documentElement.lang = language;
  }, [language]);
  useEffect(() => {
    document.title = brand || t("app");
  }, [brand, t]);
  function fail(e: unknown) {
    setError(e instanceof ApiError ? e.code : "internal");
  }
  async function loadOrgs() {
    const v = await api<Organization[]>("/organizations");
    setOrgs(v);
  }
  async function loadOrg(id: string) {
    const [o, a, s] = await Promise.all([
      api<Organization>("/organizations/" + id),
      api<Assistant[]>("/organizations/" + id + "/assistants"),
      api<Site[]>("/organizations/" + id + "/sites"),
    ]);
    setOrg(o);
    setAssistants(a);
    setSites(s);
  }
  useEffect(() => {
    void (async () => {
      try {
        const c = await api<{ brand_name: string; default_locale: Locale }>(
          "/config",
        );
        setBrand(c.brand_name);
        const browserLocale = navigator.language.split("-")[0];
        if (
          !localStorage.getItem("locale") &&
          !locales.includes(browserLocale as Locale)
        ) {
          await i18n.changeLanguage(c.default_locale);
        }
        try {
          const u = await api<User>("/auth/me");
          setUser(u);
          await i18n.changeLanguage(u.preferred_locale);
          await loadOrgs();
        } catch (e) {
          if (!(e instanceof ApiError && e.status === 401)) throw e;
        }
      } catch (e) {
        fail(e);
      } finally {
        setBoot(false);
      }
    })();
  }, []);
  async function action(fn: () => Promise<void>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await fn();
    } catch (e) {
      fail(e);
    } finally {
      setBusy(false);
    }
  }
  async function changeLanguage(value: string) {
    await action(async () => {
      if (user) {
        const u = await api<User>("/profile", "PATCH", {
          preferred_locale: value,
        });
        setUser(u);
      }
      await i18n.changeLanguage(value);
      localStorage.setItem("locale", value);
    });
  }
  function values(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    return Object.fromEntries(new FormData(e.currentTarget)) as Record<
      string,
      string
    >;
  }
  const languageSelect = (value = language, name = "default_locale") => (
    <select key={value} name={name} defaultValue={value}>
      {locales.map((l) => (
        <option key={l} value={l}>
          {t("languages." + l)}
        </option>
      ))}
    </select>
  );
  const field = (key: string, child: React.ReactNode) => (
    <label>
      {t(key)}
      {child}
    </label>
  );
  const input = (name: string, type = "text", value = "", required = true) => (
    <input
      name={name}
      type={type}
      defaultValue={value}
      required={required}
      maxLength={name === "description" ? 4000 : 160}
    />
  );
  const date = (value: string) =>
    new Intl.DateTimeFormat(language, {
      dateStyle: "medium",
      ...(org ? { timeZone: org.timezone } : {}),
    }).format(new Date(value));
  const writable = org?.role === "owner" || org?.role === "admin";
  function showDialog(kind: string) {
    setError("");
    setDialog(kind);
  }
  if (boot)
    return (
      <main className="boot" role="status">
        {t("loading")}
      </main>
    );
  return (
    <>
      <header>
        <a
          className="brand"
          href="#"
          onClick={(e) => {
            e.preventDefault();
            setOrg(null);
            setPage("home");
          }}
        >
          <span className="mark">
            s<span>·</span>
          </span>
          {brand || t("app")}
        </a>
        <div className="header-actions">
          <select
            aria-label={t("language")}
            value={language}
            disabled={busy}
            onChange={(e) => void changeLanguage(e.target.value)}
          >
            {locales.map((l) => (
              <option key={l} value={l}>
                {t("languages." + l)}
              </option>
            ))}
          </select>
          {user && (
            <button
              className="avatar"
              onClick={() => {
                setPage("profile");
                setDialog("");
              }}
              aria-label={t("profile")}
            >
              {(user.name || user.email)[0].toUpperCase()}
            </button>
          )}
        </div>
      </header>
      {error && (
        <div className="alert" role="alert">
          {t("errors." + error, { defaultValue: t("errors.internal") })}
          <button aria-label={t("close")} onClick={() => setError("")}>
            ×
          </button>
        </div>
      )}
      {notice && (
        <div className="notice" role="status">
          {t(notice)}
        </div>
      )}
      {!user ? (
        <main className="auth-layout">
          <section className="intro">
            <div className="eyebrow">{t("workspace")}</div>
            <h1>{t("heroTitle")}</h1>
            <p>{t("heroBody")}</p>
            <div className="intro-card">
              <span className="orb" />
              <div>
                <strong>{t("foundationTitle")}</strong>
                <p>{t("foundationBody")}</p>
              </div>
            </div>
          </section>
          <section className="auth card">
            <h2>{t(register ? "register" : "login")}</h2>
            <p className="muted">
              {t(register ? "registerHint" : "loginHint")}
            </p>
            <form
              onSubmit={(e) => {
                const v = values(e);
                void action(async () => {
                  if (register && v.password !== v.repeat)
                    throw new ApiError("password_mismatch", 422);
                  if (register) {
                    await api("/auth/register", "POST", {
                      email: v.email,
                      password: v.password,
                      preferred_locale: language,
                    });
                    setRegister(false);
                    setNotice("registrationComplete");
                    return;
                  }
                  const u = await api<User>("/auth/login", "POST", {
                    email: v.email,
                    password: v.password,
                  });
                  setUser(u);
                  await i18n.changeLanguage(u.preferred_locale);
                  await loadOrgs();
                });
              }}
            >
              {field(
                "email",
                <input
                  type="email"
                  name="email"
                  autoComplete="email"
                  required
                  maxLength={254}
                />,
              )}
              {field(
                "password",
                <input
                  type="password"
                  name="password"
                  autoComplete={register ? "new-password" : "current-password"}
                  minLength={register ? 12 : 1}
                  maxLength={128}
                  required
                />,
              )}
              {register &&
                field(
                  "repeatPassword",
                  <input
                    type="password"
                    name="repeat"
                    autoComplete="new-password"
                    minLength={12}
                    maxLength={128}
                    required
                  />,
                )}
              {register && <small>{t("passwordHint")}</small>}
              <button className="primary" disabled={busy}>
                {t(busy ? "saving" : register ? "register" : "login")}
              </button>
            </form>
            <button
              className="text-button"
              disabled={busy}
              onClick={() => {
                setRegister(!register);
                setError("");
              }}
            >
              {t(register ? "haveAccount" : "needAccount")}
            </button>
          </section>
        </main>
      ) : (
        <div className="app-layout">
          <aside>
            <div className="workspace-label">{t("workspace")}</div>
            <button
              className={!org && page === "home" ? "nav active" : "nav"}
              onClick={() => {
                setOrg(null);
                setPage("home");
                setDialog("");
              }}
            >
              {t("organizations")}
            </button>
            {org && (
              <>
                <div className="org-label">{org.name}</div>
                {[
                  "home",
                  "assistants",
                  "knowledge",
                  "preview",
                  "sites",
                  "widget",
                  ...(org.role === "owner" ? ["settings"] : []),
                ].map((p) => (
                  <button
                    key={p}
                    className={page === p ? "nav active" : "nav"}
                    onClick={() => {
                      setPage(p);
                      setDialog("");
                    }}
                  >
                    {t(p === "widget" ? "widgetTitle" : p)}
                  </button>
                ))}
              </>
            )}
          </aside>
          <main className="content">
            {page === "profile" ? (
              <>
                <div className="eyebrow">{t("account")}</div>
                <h1>{t("profile")}</h1>
                <section className="card form-card">
                  <form
                    onSubmit={(e) => {
                      const v = values(e);
                      void action(async () => {
                        const u = await api<User>("/profile", "PATCH", {
                          name: v.name,
                          preferred_locale: v.default_locale,
                        });
                        setUser(u);
                        await i18n.changeLanguage(u.preferred_locale);
                        localStorage.setItem("locale", u.preferred_locale);
                        setNotice("saved");
                      });
                    }}
                  >
                    {field("name", input("name", "text", user.name, false))}
                    {field("email", <input value={user.email} disabled />)}
                    {field("language", languageSelect(user.preferred_locale))}
                    <button className="primary" disabled={busy}>
                      {t("save")}
                    </button>
                  </form>
                  <hr />
                  <button
                    className="danger"
                    disabled={busy}
                    onClick={() =>
                      void action(async () => {
                        await api("/auth/logout", "POST", {});
                        setUser(null);
                        setOrg(null);
                        setOrgs([]);
                        setPage("home");
                        setDialog("");
                      })
                    }
                  >
                    {t("logout")}
                  </button>
                </section>
              </>
            ) : !org ? (
              <>
                <div className="eyebrow">{t("workspace")}</div>
                <div className="page-heading">
                  <div>
                    <h1>{t("welcome", { name: user.name || user.email })}</h1>
                    <p>{t("organizationsHint")}</p>
                  </div>
                  <button
                    className="primary"
                    onClick={() => showDialog("organization")}
                  >
                    {t("createOrganization")}
                  </button>
                </div>
                <div className="grid">
                  {orgs.map((o) => (
                    <button
                      className="card org-card"
                      key={o.id}
                      disabled={busy}
                      onClick={() =>
                        void action(async () => {
                          await loadOrg(o.id);
                          setPage("home");
                          setDialog("");
                        })
                      }
                    >
                      <span className="company-icon">{o.name[0]}</span>
                      <h2>{o.name}</h2>
                      <span className="badge">{t("roles." + o.role)}</span>
                      <p className="muted">{date(o.created_at)}</p>
                      <span className="link">{t("openWorkspace")} →</span>
                    </button>
                  ))}
                </div>
                {!orgs.length && (
                  <section className="empty card">
                    <h2>{t("noOrganizations")}</h2>
                    <p>{t("organizationsHint")}</p>
                  </section>
                )}
              </>
            ) : (
              <>
                <div className="eyebrow">
                  {org.name} · {t("roles." + org.role)}
                </div>
                <div className="page-heading">
                  <div>
                    <h1>{t(page === "widget" ? "widgetTitle" : page)}</h1>
                    <p>{t(page + "Hint")}</p>
                  </div>
                  {writable && (page === "assistants" || page === "sites") && (
                    <button
                      className="primary"
                      onClick={() => {
                        setEditing(null);
                        showDialog(
                          page === "assistants" ? "assistant" : "site",
                        );
                      }}
                    >
                      {t(page === "assistants" ? "createAssistant" : "addSite")}
                    </button>
                  )}
                </div>
                {page === "home" && (
                  <>
                    <div className="grid stats">
                      <section className="card">
                        <span>{t("assistants")}</span>
                        <strong>
                          {new Intl.NumberFormat(language).format(
                            assistants.length,
                          )}
                        </strong>
                      </section>
                      <section className="card">
                        <span>{t("sites")}</span>
                        <strong>
                          {new Intl.NumberFormat(language).format(sites.length)}
                        </strong>
                      </section>
                      <section className="card">
                        <span>{t("timezone")}</span>
                        <strong className="small-value">{org.timezone}</strong>
                      </section>
                    </div>
                    <section className="card next-step">
                      <div className="orb" />
                      <div>
                        <h2>{t("foundationTitle")}</h2>
                        <p>{t("draftNotice")}</p>
                        <button
                          className="primary"
                          onClick={() => setPage("assistants")}
                        >
                          {t("assistants")} →
                        </button>
                      </div>
                    </section>
                  </>
                )}
                {page === "assistants" && (
                  <>
                    <p className="info">{t("draftNotice")}</p>
                    <div className="grid">
                      {assistants.map((a) => (
                        <section className="card" key={a.id}>
                          <span className="badge">{t("draft")}</span>
                          <h2>{a.name}</h2>
                          <button
                            onClick={() => {
                              setKnowledgeAssistant(a.id);
                              setPage("preview");
                            }}
                          >
                            {t("preview")}
                          </button>
                          <button
                            onClick={() => {
                              setKnowledgeAssistant(a.id);
                              setPage("knowledge");
                            }}
                          >
                            {t("knowledge")}
                          </button>
                          <p className="description">
                            {a.description || t("noDescription")}
                          </p>
                          <p className="muted">
                            {t("languages." + a.default_locale)} ·{" "}
                            {date(a.created_at)}
                          </p>
                          {writable && (
                            <button
                              onClick={() => {
                                setEditing(a);
                                showDialog("assistant");
                              }}
                            >
                              {t("edit")}
                            </button>
                          )}
                        </section>
                      ))}
                    </div>
                    {!assistants.length && (
                      <section className="empty card">
                        <h2>{t("noAssistants")}</h2>
                        <p>{t("assistantsHint")}</p>
                      </section>
                    )}
                  </>
                )}
                {page === "sites" && (
                  <>
                    <div className="site-list">
                      {sites.map((s) => (
                        <section className="card site-row" key={s.id}>
                          <div>
                            <h2>{s.name}</h2>
                            <p>{s.domain}</p>
                            <small className="muted">
                              {
                                assistants.find((a) => a.id === s.assistant_id)
                                  ?.name
                              }
                            </small>
                          </div>
                          <span className="badge">{t("unverified")}</span>
                          <button
                            onClick={() => {
                              setWidgetSite(s.id);
                              setPage("widget");
                            }}
                          >
                            {t("widget.appearance")}
                          </button>
                          {writable && (
                            <button
                              className="danger"
                              onClick={() => setSiteDelete(s)}
                            >
                              {t("delete")}
                            </button>
                          )}
                        </section>
                      ))}
                    </div>
                    {!sites.length && (
                      <section className="empty card">
                        <h2>{t("noSites")}</h2>
                        <p>{t("sitesHint")}</p>
                      </section>
                    )}
                  </>
                )}
                {page === "knowledge" && (
                  <Knowledge
                    key={org.id + knowledgeAssistant}
                    organizationId={org.id}
                    assistants={assistants}
                    writable={writable}
                    initialAssistant={knowledgeAssistant}
                  />
                )}
                {page === "preview" && (
                  <PreviewChat
                    key={org.id + knowledgeAssistant}
                    organizationId={org.id}
                    assistants={assistants}
                    writable={writable}
                    initialAssistant={knowledgeAssistant}
                  />
                )}
                {page === "widget" && (
                  <WidgetSettings
                    key={org.id + widgetSite}
                    organizationId={org.id}
                    sites={sites}
                    initialSite={widgetSite}
                    writable={writable}
                  />
                )}
                {page === "settings" && org.role === "owner" && (
                  <section className="card form-card">
                    <OrganizationForm
                      key={org.id}
                      org={org}
                      disabled={busy}
                      onSubmit={(v) =>
                        void action(async () => {
                          await api("/organizations/" + org.id, "PATCH", v);
                          await loadOrg(org.id);
                          await loadOrgs();
                          setNotice("saved");
                        })
                      }
                    />
                  </section>
                )}
              </>
            )}
          </main>
        </div>
      )}
      {dialog && user && (
        <div className="modal-backdrop">
          <section
            role="dialog"
            aria-modal="true"
            aria-label={t(
              dialog === "organization"
                ? "createOrganization"
                : dialog === "site"
                  ? "addSite"
                  : editing
                    ? "editAssistant"
                    : "createAssistant",
            )}
            className="modal card"
          >
            <div className="modal-heading">
              <h2>
                {t(
                  dialog === "organization"
                    ? "createOrganization"
                    : dialog === "site"
                      ? "addSite"
                      : editing
                        ? "editAssistant"
                        : "createAssistant",
                )}
              </h2>
              <button
                disabled={busy}
                onClick={() => setDialog("")}
                aria-label={t("close")}
              >
                ×
              </button>
            </div>
            {error && (
              <p className="form-error" role="alert">
                {t("errors." + error, { defaultValue: t("errors.internal") })}
              </p>
            )}
            {dialog === "organization" ? (
              <OrganizationForm
                disabled={busy}
                onSubmit={(v) =>
                  void action(async () => {
                    const o = await api<Organization>(
                      "/organizations",
                      "POST",
                      v,
                    );
                    await loadOrgs();
                    await loadOrg(o.id);
                    setDialog("");
                    setPage("home");
                  })
                }
              />
            ) : dialog === "assistant" ? (
              <form
                key={editing?.id || "new"}
                onSubmit={(e) => {
                  const v = values(e);
                  void action(async () => {
                    await api(
                      "/organizations/" +
                        org!.id +
                        "/assistants" +
                        (editing ? "/" + editing.id : ""),
                      editing ? "PATCH" : "POST",
                      v,
                    );
                    await loadOrg(org!.id);
                    setDialog("");
                  });
                }}
              >
                {field("name", input("name", "text", editing?.name))}
                {field(
                  "description",
                  <textarea
                    name="description"
                    defaultValue={editing?.description}
                    maxLength={4000}
                  />,
                )}
                {field(
                  "defaultLanguage",
                  languageSelect(
                    editing?.default_locale || org?.default_locale,
                  ),
                )}
                {field(
                  "languagePolicy",
                  <select
                    name="language_policy"
                    defaultValue={editing?.language_policy || "visitor"}
                  >
                    <option value="visitor">{t("visitorLanguage")}</option>
                    <option value="fixed">{t("fixedLanguage")}</option>
                  </select>,
                )}
                <button className="primary" disabled={busy}>
                  {t(busy ? "saving" : "save")}
                </button>
              </form>
            ) : (
              <form
                onSubmit={(e) => {
                  const v = values(e);
                  void action(async () => {
                    await api(
                      "/organizations/" + org!.id + "/sites",
                      "POST",
                      v,
                    );
                    await loadOrg(org!.id);
                    setDialog("");
                  });
                }}
              >
                {field("name", input("name"))}
                {field(
                  "domain",
                  <input
                    name="domain"
                    required
                    placeholder={t("domainExample")}
                    maxLength={253}
                  />,
                )}
                {field(
                  "assistant",
                  <select name="assistant_id" required>
                    <option value="">{t("selectAssistant")}</option>
                    {assistants.map((a) => (
                      <option value={a.id} key={a.id}>
                        {a.name}
                      </option>
                    ))}
                  </select>,
                )}
                {!assistants.length && (
                  <p className="info">{t("createAssistantFirst")}</p>
                )}
                <p className="muted">{t("siteNotice")}</p>
                <button
                  className="primary"
                  disabled={busy || !assistants.length}
                >
                  {t(busy ? "saving" : "save")}
                </button>
              </form>
            )}
          </section>
        </div>
      )}
      {siteDelete && (
        <div className="modal-backdrop">
          <section
            className="modal card"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="delete-title"
          >
            <h2 id="delete-title">{t("confirmDelete")}</h2>
            <p>{siteDelete.domain}</p>
            <div className="actions">
              <button disabled={busy} onClick={() => setSiteDelete(null)}>
                {t("cancel")}
              </button>
              <button
                className="danger"
                disabled={busy}
                onClick={() =>
                  void action(async () => {
                    await api(
                      "/organizations/" + org!.id + "/sites/" + siteDelete.id,
                      "DELETE",
                      {},
                    );
                    await loadOrg(org!.id);
                    setSiteDelete(null);
                  })
                }
              >
                {t("delete")}
              </button>
            </div>
          </section>
        </div>
      )}
      <footer>{t("footer")}</footer>
    </>
  );
}
function OrganizationForm({
  org,
  disabled,
  onSubmit,
}: {
  org?: Organization;
  disabled: boolean;
  onSubmit: (v: Record<string, string>) => void;
}) {
  const { t } = useTranslation();
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit(
          Object.fromEntries(new FormData(e.currentTarget)) as Record<
            string,
            string
          >,
        );
      }}
    >
      <label>
        {t("organizationName")}
        <input name="name" defaultValue={org?.name} required maxLength={160} />
      </label>
      <div className="form-grid">
        <label>
          {t("country")}
          <input
            name="country"
            defaultValue={org?.country || "CZ"}
            required
            pattern="[A-Za-z]{2}"
            maxLength={2}
          />
        </label>
        <label>
          {t("timezone")}
          <input
            name="timezone"
            defaultValue={
              org?.timezone ||
              Intl.DateTimeFormat().resolvedOptions().timeZone ||
              "UTC"
            }
            required
            maxLength={100}
          />
        </label>
      </div>
      <label>
        {t("defaultLanguage")}
        <select
          name="default_locale"
          defaultValue={org?.default_locale || i18n.resolvedLanguage}
        >
          {locales.map((l) => (
            <option key={l} value={l}>
              {t("languages." + l)}
            </option>
          ))}
        </select>
      </label>
      <button className="primary" disabled={disabled}>
        {t(disabled ? "saving" : "save")}
      </button>
    </form>
  );
}
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
