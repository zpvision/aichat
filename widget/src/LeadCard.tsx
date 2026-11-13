import { useEffect, useRef, useState } from "react";
import type { Locale, Session } from "./types";
import { request } from "./transport";
import { leadText } from "./lead-locales";
type Field = {
  key: string;
  label: string;
  labels: Record<string, string>;
  type: string;
  required: boolean;
  active: boolean;
  options: string[];
  max_length: number;
  min: number | null;
  max: number | null;
  integer: boolean;
};
type Collection = {
  state: string;
  version: number;
  conversation_version: number;
  values: Record<string, { value: string | number; valid: boolean }>;
  fields: Field[];
  missing: string[];
  paused: boolean;
  enabled: boolean;
  privacy_url: string;
  error_code: string;
  success_messages: Record<string, string>;
};
export function LeadCard({
  enabled,
  widgetId,
  parentOrigin,
  locale,
  sessionId,
  getSession,
  ensureSession,
  isVisible,
  disabled,
  mode,
}: {
  enabled: boolean;
  widgetId: string;
  parentOrigin: string;
  locale: Locale;
  sessionId: string;
  getSession: () => Session | null;
  ensureSession: () => Promise<Session>;
  isVisible: () => boolean;
  disabled: boolean;
  mode: string;
}) {
  const [data, setData] = useState<Collection | null>(null),
    [edit, setEdit] = useState(false),
    [values, setValues] = useState<Record<string, string | number | null>>({}),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(false);
  const running = useRef(false),
    mutating = useRef(false),
    epoch = useRef(0);
  const t = (k: string) => leadText(locale, k),
    label = (f: Field) => f.labels?.[locale] || f.label || t("field_" + f.key);
  useEffect(() => {
    const current = ++epoch.current;
    setData(null);
    setEdit(false);
    setError(false);
    let active = true;
    const load = async () => {
      const s = getSession();
      if (
        !s ||
        document.hidden ||
        !isVisible() ||
        running.current ||
        mutating.current
      )
        return;
      running.current = true;
      try {
        const d = await request<Collection>(
          widgetId,
          parentOrigin,
          `/sessions/${s.id}/collection`,
          "GET",
          s,
        );
        if (active && current === epoch.current)
          setData((old) => (!old || old.version <= d.version ? d : old));
      } catch {
        if (active) setError(true);
      } finally {
        running.current = false;
      }
    };
    void load();
    const timer = setInterval(() => void load(), 3000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [sessionId, widgetId, parentOrigin]);
  async function act(kind: string) {
    if (busy || disabled) return;
    setBusy(true);
    mutating.current = true;
    setError(false);
    try {
      const s = await ensureSession();
      const d = await request<Collection>(
        widgetId,
        parentOrigin,
        `/sessions/${s.id}/collection`,
        "GET",
        s,
      );
      if (
        data &&
        (data.version !== d.version ||
          data.conversation_version !== d.conversation_version) &&
        kind !== "start"
      ) {
        setData(d);
        setError(true);
        return;
      }
      const updated = await request<Collection>(
        widgetId,
        parentOrigin,
        `/sessions/${s.id}/collection/${kind}`,
        "POST",
        s,
        {
          version: d.version,
          conversation_version: d.conversation_version,
          ...(kind === "edit" ? { values } : {}),
        },
      );
      setData(updated);
      setEdit(false);
    } catch {
      setError(true);
    } finally {
      setBusy(false);
      mutating.current = false;
    }
  }
  const state = data?.state || "idle",
    active = state === "collecting" || state === "confirmation",
    paused = data?.paused || mode !== "ai_active";
  if (!data && !enabled) return null;
  return (
    <section className="lead-card" aria-label={t("request")}>
      {error && <p role="alert">{t("error")}</p>}
      {(!data || data.enabled) &&
        ["idle", "canceled"].includes(state) &&
        mode === "ai_active" && (
          <button
            type="button"
            className="request-human"
            disabled={busy || disabled}
            onClick={() => void act("start")}
          >
            {t("start")}
          </button>
        )}
      {active && data && (
        <>
          <h2>{t("state_" + state)}</h2>
          {paused && <p role="status">{t("paused")}</p>}
          {data.error_code && <p role="alert">{t("error")}</p>}
          {Object.entries(data.values).map(([key, v]) => (
            <p className="lead-value" key={key}>
              <span>{label(data.fields.find((f) => f.key === key)!)}</span>
              <strong>{String(v.value)}</strong>
              {!v.valid && <small>{t("invalid")}</small>}
            </p>
          ))}
          {data.missing.length > 0 && (
            <p className="lead-missing">
              {t("missing")}:{" "}
              {data.missing
                .map((k) =>
                  k === "$contact"
                    ? t("contactRule")
                    : label(data.fields.find((f) => f.key === k)!),
                )
                .join(", ")}
            </p>
          )}
          {edit && !paused ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void act("edit");
              }}
            >
              {data.fields.map((f) => (
                <label key={f.key}>
                  {label(f)}
                  {f.required ? " *" : ""}
                  {f.type === "select" ? (
                    <select
                      value={values[f.key] ?? ""}
                      onChange={(e) =>
                        setValues({
                          ...values,
                          [f.key]: e.target.value || null,
                        })
                      }
                    >
                      <option value="">—</option>
                      {f.options.map((o) => (
                        <option key={o}>{o}</option>
                      ))}
                    </select>
                  ) : (
                    <input
                      value={values[f.key] ?? ""}
                      type={
                        f.type === "number"
                          ? "number"
                          : f.type === "email"
                            ? "email"
                            : f.type === "phone"
                              ? "tel"
                              : "text"
                      }
                      maxLength={f.type === "text" ? f.max_length || 500 : 2000}
                      min={f.min ?? undefined}
                      max={f.max ?? undefined}
                      step={f.integer ? 1 : "any"}
                      onChange={(e) =>
                        setValues({
                          ...values,
                          [f.key]:
                            e.target.value === ""
                              ? null
                              : f.type === "number"
                                ? Number(e.target.value)
                                : e.target.value,
                        })
                      }
                    />
                  )}
                </label>
              ))}
              <div className="lead-buttons">
                <button
                  type="button"
                  disabled={busy || disabled}
                  onClick={() => void act("edit")}
                >
                  {t("saveData")}
                </button>
                <button type="button" onClick={() => setEdit(false)}>
                  {t("back")}
                </button>
              </div>
            </form>
          ) : (
            !paused && (
              <div className="lead-buttons">
                <button
                  type="button"
                  disabled={busy || disabled}
                  onClick={() => {
                    setValues(
                      Object.fromEntries(
                        Object.entries(data.values).map(([k, v]) => [
                          k,
                          v.value,
                        ]),
                      ),
                    );
                    setEdit(true);
                  }}
                >
                  {t("editData")}
                </button>
                {state === "confirmation" && (
                  <button
                    type="button"
                    className="primary"
                    disabled={busy || disabled}
                    onClick={() => void act("submit")}
                  >
                    {t("confirm")}
                  </button>
                )}
              </div>
            )
          )}
          <div className="lead-buttons">
            <button
              type="button"
              disabled={busy || disabled}
              onClick={() => void act("cancel")}
            >
              {t("cancel")}
            </button>
          </div>
          {data.privacy_url && (
            <a
              href={data.privacy_url}
              target="_blank"
              rel="noopener noreferrer"
            >
              {t("privacy")}
            </a>
          )}
        </>
      )}
      {state === "submitted" && (
        <p role="status">
          {data?.success_messages?.[locale] ||
            data?.success_messages?.en ||
            t("success")}
        </p>
      )}
    </section>
  );
}
