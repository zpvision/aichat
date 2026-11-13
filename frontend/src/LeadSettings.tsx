import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError, locales, type Assistant } from "./api";
import type { LeadField, LeadSettings as Settings } from "./leads-api";
import { LeadFields, useFieldLabel } from "./LeadFields";
export function LeadSettings({
  organizationId,
  assistants,
}: {
  organizationId: string;
  assistants: Assistant[];
}) {
  const { t } = useTranslation(),
    label = useFieldLabel(),
    tr = (k: string) => t("lead." + k);
  const [assistant, setAssistant] = useState(assistants[0]?.id || ""),
    [data, setData] = useState<Settings | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [saved, setSaved] = useState(false);
  const savedKeys = useRef<Set<string>>(new Set());
  const base = `/organizations/${organizationId}/assistants/${assistant}/lead-settings`;
  useEffect(() => {
    let active = true;
    setData(null);
    setError("");
    if (assistant)
      void api<Settings>(base)
        .then((d) => {
          if (active) {
            savedKeys.current = new Set(d.fields.map((f) => f.key));
            setData(d);
          }
        })
        .catch((e) => {
          if (active) setError(e instanceof ApiError ? e.code : "internal");
        });
    return () => {
      active = false;
    };
  }, [base, assistant]);
  function field(i: number, patch: Partial<LeadField>) {
    setSaved(false);
    setData((d) =>
      d
        ? {
            ...d,
            fields: d.fields.map((f, n) => (i === n ? { ...f, ...patch } : f)),
          }
        : d,
    );
  }
  async function save() {
    if (!data) return;
    setBusy(true);
    setError("");
    setSaved(false);
    try {
      const d = await api<Settings>(base, "PUT", data);
      savedKeys.current = new Set(d.fields.map((f) => f.key));
      setData(d);
      setSaved(true);
    } catch (e) {
      setError(e instanceof ApiError ? e.code : "internal");
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="lead-settings">
      <label>
        {t("assistant")}
        <select
          aria-label={t("assistant")}
          value={assistant}
          onChange={(e) => setAssistant(e.target.value)}
        >
          {assistants.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      </label>
      {!assistant && <p>{t("noAssistants")}</p>}
      {error && (
        <p className="alert" role="alert">
          {t("errors." + error, { defaultValue: t("errors.internal") })}
        </p>
      )}
      {saved && (
        <p className="notice" role="status">
          {t("saved")}
        </p>
      )}
      {data && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void save();
          }}
        >
          <div className="card">
            <h2>{tr("scenario")}</h2>
            <p className="muted">{tr("consultingHint")}</p>
            <label className="check">
              <input
                type="checkbox"
                checked={data.enabled}
                onChange={(e) => {
                  setSaved(false);
                  setData({ ...data, enabled: e.target.checked });
                }}
              />
              {tr("enabled")}
            </label>
            <label>
              {tr("scenarioName")}
              <input
                maxLength={120}
                value={data.title}
                onChange={(e) => setData({ ...data, title: e.target.value })}
              />
            </label>
            <label>
              {tr("goal")}
              <textarea
                maxLength={1500}
                value={data.goal}
                onChange={(e) => setData({ ...data, goal: e.target.value })}
              />
            </label>
            <label className="check">
              <input
                type="checkbox"
                checked={data.require_contact}
                onChange={(e) =>
                  setData({
                    ...data,
                    require_contact: e.target.checked,
                    fields: data.fields.map((f) =>
                      ["email", "phone"].includes(f.type) && e.target.checked
                        ? { ...f, required: false }
                        : f,
                    ),
                  })
                }
              />
              {tr("contactRule")}
            </label>
            <p className="muted">{tr("privacyRequired")}</p>
          </div>
          <h2>{tr("fields")}</h2>
          {data.fields.map((f, i) => (
            <details
              className="card lead-field"
              key={i}
              open={i === data.fields.length - 1}
            >
              <summary>
                {label(f)} · {tr("type_" + f.type)}
              </summary>
              <div className="lead-form-grid">
                <label>
                  {tr("key")}
                  <input
                    required
                    pattern="[a-z][a-z0-9_]{0,39}"
                    value={f.key}
                    readOnly={savedKeys.current.has(f.key)}
                    onChange={(e) => field(i, { key: e.target.value })}
                  />
                </label>
                <label>
                  {tr("label")}
                  <input
                    value={f.label}
                    maxLength={100}
                    onChange={(e) => field(i, { label: e.target.value })}
                  />
                </label>
                <label>
                  {tr("type")}
                  <select
                    aria-label={tr("type")}
                    value={f.type}
                    onChange={(e) =>
                      field(i, { type: e.target.value, required: false })
                    }
                  >
                    {[
                      "text",
                      "email",
                      "phone",
                      "number",
                      "select",
                      "inn_ru",
                    ].map((k) => (
                      <option key={k} value={k}>
                        {tr("type_" + k)}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  {tr("order")}
                  <input
                    type="number"
                    min={0}
                    max={1000}
                    value={f.position}
                    onChange={(e) =>
                      field(i, { position: Number(e.target.value) })
                    }
                  />
                </label>
              </div>
              <label>
                {tr("description")}
                <textarea
                  maxLength={400}
                  value={f.description}
                  onChange={(e) => field(i, { description: e.target.value })}
                />
              </label>
              <div className="lead-form-grid">
                {locales.map((l) => (
                  <label key={l}>
                    {t("languages." + l)}
                    <input
                      maxLength={100}
                      value={f.labels?.[l] || ""}
                      onChange={(e) =>
                        field(i, {
                          labels: { ...f.labels, [l]: e.target.value },
                        })
                      }
                    />
                  </label>
                ))}
              </div>
              <div className="lead-actions">
                <label className="check">
                  <input
                    type="checkbox"
                    checked={f.active}
                    onChange={(e) => field(i, { active: e.target.checked })}
                  />
                  {tr("active")}
                </label>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={f.required}
                    disabled={
                      data.require_contact &&
                      ["email", "phone"].includes(f.type)
                    }
                    onChange={(e) => field(i, { required: e.target.checked })}
                  />
                  {tr("required")}
                </label>
              </div>
              {f.type === "text" && (
                <div className="lead-form-grid">
                  <label>
                    {tr("minLength")}
                    <input
                      type="number"
                      min={0}
                      max={2000}
                      value={f.min_length}
                      onChange={(e) =>
                        field(i, { min_length: Number(e.target.value) })
                      }
                    />
                  </label>
                  <label>
                    {tr("maxLength")}
                    <input
                      type="number"
                      min={1}
                      max={2000}
                      value={f.max_length}
                      onChange={(e) =>
                        field(i, { max_length: Number(e.target.value) })
                      }
                    />
                  </label>
                </div>
              )}
              {f.type === "number" && (
                <>
                  <div className="lead-form-grid">
                    {(["min", "max"] as const).map((k) => (
                      <label key={k}>
                        {tr(k)}
                        <input
                          type="number"
                          value={f[k] ?? ""}
                          min={-1e9}
                          max={1e9}
                          onChange={(e) =>
                            field(i, {
                              [k]:
                                e.target.value === ""
                                  ? null
                                  : Number(e.target.value),
                            })
                          }
                        />
                      </label>
                    ))}
                  </div>
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={f.integer}
                      onChange={(e) => field(i, { integer: e.target.checked })}
                    />
                    {tr("integer")}
                  </label>
                </>
              )}
              {f.type === "select" && (
                <label>
                  {tr("options")}
                  <textarea
                    value={(f.options || []).join("\n")}
                    onChange={(e) =>
                      field(i, { options: e.target.value.split("\n") })
                    }
                  />
                </label>
              )}
              <button
                type="button"
                className="danger"
                onClick={() =>
                  setData({
                    ...data,
                    fields: data.fields.filter((_, n) => n !== i),
                  })
                }
              >
                {tr("removeField")}
              </button>
            </details>
          ))}
          <div className="lead-actions">
            <button
              type="button"
              disabled={data.fields.length >= 12}
              onClick={() =>
                setData({
                  ...data,
                  fields: [
                    ...data.fields,
                    {
                      key: "",
                      label: "",
                      labels: {},
                      description: "",
                      type: "text",
                      required: false,
                      active: true,
                      position: data.fields.length,
                      min_length: 0,
                      max_length: 500,
                      options: [],
                      min: null,
                      max: null,
                      integer: false,
                    },
                  ],
                })
              }
            >
              {tr("addField")}
            </button>
            <button className="primary" disabled={busy}>
              {t(busy ? "saving" : "save")}
            </button>
          </div>
          <details className="card">
            <summary>{tr("successMessages")}</summary>
            {locales.map((l) => (
              <label key={l}>
                {t("languages." + l)}
                <textarea
                  maxLength={400}
                  value={data.success_messages?.[l] || ""}
                  onChange={(e) =>
                    setData({
                      ...data,
                      success_messages: {
                        ...data.success_messages,
                        [l]: e.target.value,
                      },
                    })
                  }
                />
              </label>
            ))}
          </details>
          <details className="card">
            <summary>{tr("preview")}</summary>
            <p>{tr("confirmationHint")}</p>
            <LeadFields fields={data.fields} values={{}} onChange={() => {}} />
          </details>
        </form>
      )}
    </section>
  );
}
