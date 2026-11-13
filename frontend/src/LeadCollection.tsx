import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "./api";
import type { Collection } from "./leads-api";
import { LeadFields, useFieldLabel } from "./LeadFields";
export function LeadCollection({
  organizationId,
  conversationId,
  canWrite,
  onLead,
}: {
  organizationId: string;
  conversationId: string;
  canWrite: boolean;
  onLead?: (id: string) => void;
}) {
  const { t } = useTranslation(),
    label = useFieldLabel();
  const tr = (k: string) => t("lead." + k);
  const [data, setData] = useState<Collection | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [edit, setEdit] = useState(false),
    [values, setValues] = useState<Record<string, string | number | null>>({});
  const base = `/organizations/${organizationId}/conversations/${conversationId}/collection`;
  useEffect(() => {
    let active = true,
      running = false;
    setData(null);
    setEdit(false);
    const load = async () => {
      if (document.hidden || running) return;
      running = true;
      try {
        const d = await api<Collection>(base);
        if (active) setData(d);
      } catch (e) {
        if (active) setError(e instanceof ApiError ? e.code : "internal");
      } finally {
        running = false;
      }
    };
    void load();
    const timer = setInterval(() => void load(), 3000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [base]);
  async function act(kind: string) {
    if (!data) return;
    setBusy(true);
    setError("");
    try {
      setData(
        await api<Collection>(base + "/" + kind, "POST", {
          version: data.version,
          conversation_version: data.conversation_version,
          ...(kind === "edit" ? { values } : {}),
        }),
      );
      setEdit(false);
    } catch (e) {
      setError(e instanceof ApiError ? e.code : "internal");
      setData(await api<Collection>(base).catch(() => data));
    } finally {
      setBusy(false);
    }
  }
  if (!data) return null;
  return (
    <section className="lead-collection">
      <h3>{tr("request")}</h3>
      {error && (
        <p role="alert">
          {t("errors." + error, { defaultValue: t("errors.internal") })}
        </p>
      )}
      {data.demo && <small>{tr("demo")}</small>}
      <p>{tr("state_" + data.state)}</p>
      {data.error_code && (
        <p role="alert">
          {t("errors." + data.error_code, {
            defaultValue: t("errors.internal"),
          })}
        </p>
      )}
      {Object.entries(data.values).map(([k, v]) => (
        <p key={k}>
          <strong>{label(data.fields.find((f) => f.key === k)!)}</strong>:{" "}
          {String(v.value)}
          {!v.valid && (
            <small className="alert">{t("errors." + v.error_code)}</small>
          )}
        </p>
      ))}
      {data.missing.length > 0 && (
        <p>
          {tr("missing")}:{" "}
          {data.missing
            .map((k) =>
              k === "$contact"
                ? tr("contactRule")
                : label(data.fields.find((f) => f.key === k)!),
            )
            .join(", ")}
        </p>
      )}
      {data.lead_id && onLead && (
        <button onClick={() => onLead(data.lead_id!)}>{tr("open")}</button>
      )}
      {data.enabled &&
        canWrite &&
        ["idle", "canceled"].includes(data.state) && (
          <button disabled={busy} onClick={() => void act("start")}>
            {tr("start")}
          </button>
        )}
      {canWrite && ["collecting", "confirmation"].includes(data.state) && (
        <>
          {edit ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void act("edit");
              }}
            >
              <LeadFields
                fields={data.fields}
                values={values}
                onChange={(k, v) => setValues((old) => ({ ...old, [k]: v }))}
              />
              <button disabled={busy}>{tr("saveData")}</button>
              <button type="button" onClick={() => setEdit(false)}>
                {tr("back")}
              </button>
            </form>
          ) : (
            <button
              disabled={busy}
              onClick={() => {
                setValues(
                  Object.fromEntries(
                    Object.entries(data.values).map(([k, v]) => [k, v.value]),
                  ),
                );
                setEdit(true);
              }}
            >
              {tr("editData")}
            </button>
          )}
          {data.state === "confirmation" && (
            <button
              className="primary"
              disabled={busy}
              onClick={() => void act("submit")}
            >
              {tr("confirmStaff")}
            </button>
          )}
          <button disabled={busy} onClick={() => void act("cancel")}>
            {tr("cancel")}
          </button>
        </>
      )}
      {data.privacy_url && (
        <a href={data.privacy_url} target="_blank" rel="noopener noreferrer">
          {tr("privacy")}
        </a>
      )}
      {!canWrite && ["collecting", "confirmation"].includes(data.state) && (
        <p className="muted">{tr("takeFirst")}</p>
      )}
    </section>
  );
}
