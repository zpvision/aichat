import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, locales, type Site, type Locale } from "./api";
type Copy = { title: string; subtitle: string; greeting: string };
type Theme = {
  texts: Partial<Record<Locale, Copy>>;
  primary: string;
  secondary: string;
  user_color: string;
  launcher_color: string;
  position: string;
  offset_x: number;
  offset_y: number;
  show_launcher: boolean;
  remember_session: boolean;
  allow_new: boolean;
  default_locale: Locale;
  auto_locale: boolean;
  privacy_url: string;
};
type Admin = {
  widget: {
    id: string;
    settings: Theme;
    enabled: boolean;
    origins: string[];
    verified_at: string | null;
  };
  snippet: string;
  public_url: string;
  logo_url: string;
  verification_path: string;
  verification_content: string;
  local_development: boolean;
};
export function WidgetSettings({
  organizationId,
  sites,
  initialSite,
  writable,
}: {
  organizationId: string;
  sites: Site[];
  initialSite: string;
  writable: boolean;
}) {
  const { t } = useTranslation();
  const [site, setSite] = useState(initialSite || sites[0]?.id || "");
  const [admin, setAdmin] = useState<Admin | null>(null);
  const [draft, setDraft] = useState<Theme | null>(null);
  const [origins, setOrigins] = useState("");
  const [enabled, setEnabled] = useState(false);
  const [copyLocale, setCopyLocale] = useState<Locale>("en");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const frame = useRef<HTMLIFrameElement | null>(null);
  const frameReady = useRef(false);
  const channel = useRef(crypto.randomUUID());
  const base =
    "/organizations/" + organizationId + "/sites/" + site + "/widget";
  function load(v: Admin) {
    setAdmin(v);
    setDraft(v.widget.settings);
    setOrigins(v.widget.origins.join("\n"));
    setEnabled(v.widget.enabled);
  }
  useEffect(() => {
    let active = true;
    setAdmin(null);
    frameReady.current = false;
    setDraft(null);
    setError("");
    if (!site) return;
    setBusy(true);
    void api<Admin>(base)
      .then((v) => {
        if (active) load(v);
      })
      .catch((e) => {
        if (active && !(e instanceof ApiError && e.status === 404))
          setError(e instanceof ApiError ? e.code : "internal");
      })
      .finally(() => {
        if (active) setBusy(false);
      });
    return () => {
      active = false;
    };
  }, [base, site]);
  const preview = () => {
    if (admin && draft && frameReady.current)
      frame.current?.contentWindow?.postMessage(
        {
          channel: channel.current,
          type: "preview",
          data: {
            locale: copyLocale,
            config: {
              id: "preview",
              settings: draft,
              logo_url: admin.logo_url,
              provider: "preview",
              demo: false,
              available: true,
              max_message_chars: 2000,
            },
          },
        },
        new URL(admin.public_url).origin,
      );
  };
  useEffect(() => {
    preview();
    const receive = (e: MessageEvent) => {
      if (
        admin &&
        e.source === frame.current?.contentWindow &&
        e.origin === new URL(admin.public_url).origin &&
        e.data?.channel === channel.current &&
        e.data?.type === "ready"
      ) {
        frameReady.current = true;
        preview();
      }
    };
    window.addEventListener("message", receive);
    return () => window.removeEventListener("message", receive);
  }, [admin, draft, copyLocale]);
  async function act(fn: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e.code : "internal");
    } finally {
      setBusy(false);
    }
  }
  function edit<K extends keyof Theme>(key: K, value: Theme[K]) {
    setDraft((v) => (v ? { ...v, [key]: value } : v));
  }
  const copy = draft?.texts[copyLocale] || {
    title: "",
    subtitle: "",
    greeting: "",
  };
  function editCopy(key: keyof Copy, value: string) {
    if (draft)
      edit("texts", {
        ...draft.texts,
        [copyLocale]: { ...copy, [key]: value },
      });
  }
  async function logo(file: File) {
    const data = new FormData();
    data.append("file", file);
    const r = await fetch("/api/v1" + base + "/logo", {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-CSRF-Protection": "1" },
      body: data,
    });
    const b = await r.json();
    if (!r.ok) throw new ApiError(b.error?.code || "internal", r.status);
    setAdmin(b);
  }
  return (
    <section className="widget-settings">
      <label className="assistant-picker">
        {t("widget.site")}
        <select
          aria-label={t("widget.site")}
          value={site}
          disabled={busy}
          onChange={(e) => setSite(e.target.value)}
        >
          <option value="" disabled>
            {t("widget.selectSite")}
          </option>
          {sites.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name} · {s.domain}
            </option>
          ))}
        </select>
      </label>
      {error && (
        <div className="alert" role="alert">
          {t("errors." + error, { defaultValue: t("errors.internal") })}
        </div>
      )}
      {notice && (
        <div className="notice" role="status">
          {t(notice)}
        </div>
      )}
      {!site ? (
        <section className="empty card">{t("widget.noSites")}</section>
      ) : !admin ? (
        <section className="card empty">
          <h2>{t("widget.setupTitle")}</h2>
          <p>{t("widget.setupHint")}</p>
          {writable && (
            <button
              className="primary"
              disabled={busy}
              onClick={() =>
                void act(async () => load(await api<Admin>(base, "POST", {})))
              }
            >
              {t("widget.create")}
            </button>
          )}
        </section>
      ) : (
        draft && (
          <>
            <div className="widget-editor">
              <section className="card">
                <h2>{t("widget.appearance")}</h2>
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    void act(async () => {
                      load(
                        await api<Admin>(base, "PATCH", {
                          settings: draft,
                          enabled,
                          origins: origins
                            .split("\n")
                            .map((v) => v.trim())
                            .filter(Boolean),
                        }),
                      );
                      setNotice("saved");
                    });
                  }}
                >
                  <fieldset disabled={!writable || busy}>
                    <label>
                      {t("widget.copyLanguage")}
                      <select
                        value={copyLocale}
                        onChange={(e) =>
                          setCopyLocale(e.target.value as Locale)
                        }
                      >
                        {locales.map((l) => (
                          <option key={l} value={l}>
                            {t("languages." + l)}
                          </option>
                        ))}
                      </select>
                    </label>
                    {(["title", "subtitle", "greeting"] as const).map((key) => (
                      <label key={key}>
                        {t("widget." + key)}
                        <textarea
                          aria-label={t("widget." + key)}
                          rows={key === "greeting" ? 4 : 2}
                          value={copy[key]}
                          maxLength={
                            { title: 60, subtitle: 140, greeting: 400 }[key]
                          }
                          onChange={(e) => editCopy(key, e.target.value)}
                          placeholder={t("widget.defaultCopy")}
                        />
                      </label>
                    ))}
                    <div className="widget-colors">
                      {(
                        [
                          "primary",
                          "secondary",
                          "user_color",
                          "launcher_color",
                        ] as const
                      ).map((key) => (
                        <label key={key}>
                          {t("widget.colors." + key)}
                          <input
                            aria-label={t("widget.colors." + key)}
                            type="color"
                            value={draft[key]}
                            onChange={(e) => edit(key, e.target.value)}
                          />
                        </label>
                      ))}
                    </div>
                    <label>
                      {t("widget.position")}
                      <select
                        value={draft.position}
                        onChange={(e) => edit("position", e.target.value)}
                      >
                        <option value="right">{t("widget.right")}</option>
                        <option value="left">{t("widget.left")}</option>
                      </select>
                    </label>
                    <div className="form-grid">
                      <label>
                        {t("widget.offsetX")}
                        <input
                          type="number"
                          min={8}
                          max={80}
                          value={draft.offset_x}
                          onChange={(e) =>
                            edit("offset_x", Number(e.target.value))
                          }
                        />
                      </label>
                      <label>
                        {t("widget.offsetY")}
                        <input
                          type="number"
                          min={8}
                          max={80}
                          value={draft.offset_y}
                          onChange={(e) =>
                            edit("offset_y", Number(e.target.value))
                          }
                        />
                      </label>
                    </div>
                    {(
                      [
                        "show_launcher",
                        "remember_session",
                        "allow_new",
                        "auto_locale",
                      ] as const
                    ).map((key) => (
                      <label className="checkbox-label" key={key}>
                        <input
                          type="checkbox"
                          checked={draft[key]}
                          onChange={(e) => edit(key, e.target.checked)}
                        />
                        {t("widget.behavior." + key)}
                      </label>
                    ))}
                    <label>
                      {t("defaultLanguage")}
                      <select
                        value={draft.default_locale}
                        onChange={(e) =>
                          edit("default_locale", e.target.value as Locale)
                        }
                      >
                        {locales.map((l) => (
                          <option key={l} value={l}>
                            {t("languages." + l)}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label>
                      {t("widget.privacy")}
                      <input
                        type="url"
                        maxLength={1000}
                        value={draft.privacy_url}
                        onChange={(e) => edit("privacy_url", e.target.value)}
                      />
                    </label>
                    <hr />
                    <h2>{t("widget.access")}</h2>
                    <label>
                      {t("widget.origins")}
                      <textarea
                        aria-label={t("widget.origins")}
                        value={origins}
                        onChange={(e) => setOrigins(e.target.value)}
                      />
                    </label>
                    <p className="muted">
                      {t("widget.originsHint")}
                      {admin.local_development && " " + t("widget.localHint")}
                    </p>
                    <label className="checkbox-label">
                      <input
                        type="checkbox"
                        checked={enabled}
                        onChange={(e) => setEnabled(e.target.checked)}
                      />
                      {t("widget.enabled")}
                    </label>
                    <button className="primary" disabled={busy || !writable}>
                      {t("save")}
                    </button>
                  </fieldset>
                </form>
                <hr />
                <label>
                  {t("widget.logo")}
                  <input
                    type="file"
                    accept="image/png,image/jpeg"
                    disabled={busy || !writable}
                    onChange={(e) => {
                      const f = e.target.files?.[0];
                      if (f) void act(() => logo(f));
                    }}
                  />
                </label>
                <small>{t("widget.logoHint")}</small>
              </section>
              <section className="widget-live">
                <div className="eyebrow">{t("widget.preview")}</div>
                <iframe
                  ref={frame}
                  title={t("widget.preview")}
                  src={
                    admin.public_url +
                    "/widget/preview?parent_origin=" +
                    encodeURIComponent(location.origin) +
                    "&channel=" +
                    channel.current
                  }
                  sandbox="allow-scripts allow-same-origin"
                  referrerPolicy="no-referrer"
                />
              </section>
            </div>
            <section className="card widget-install">
              <h2>{t("widget.install")}</h2>
              <p>{t("widget.installHint")}</p>
              <textarea
                aria-label={t("widget.install")}
                readOnly
                value={admin.snippet}
                onFocus={(e) => e.currentTarget.select()}
              />
              <button
                onClick={() =>
                  void act(async () => {
                    await navigator.clipboard.writeText(admin.snippet);
                    setNotice("widget.copied");
                  })
                }
              >
                {t("widget.copy")}
              </button>
              <hr />
              <h2>{t("widget.verification")}</h2>
              <p className="info">
                {t(
                  admin.widget.verified_at
                    ? "widget.verified"
                    : "widget.unverified",
                )}
              </p>
              <p>{t("widget.verifyHint")}</p>
              <code>{admin.verification_path}</code>
              <textarea
                aria-label={t("widget.verificationContent")}
                readOnly
                value={admin.verification_content}
              />
              {writable && (
                <button
                  disabled={busy}
                  onClick={() =>
                    void act(async () => {
                      load(await api<Admin>(base + "/verify", "POST", {}));
                      setNotice("widget.verified");
                    })
                  }
                >
                  {t("widget.verify")}
                </button>
              )}
            </section>
          </>
        )
      )}
    </section>
  );
}
