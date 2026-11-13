import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "./api";
type Period = {
  id: string;
  starts_at: string;
  ends_at: string;
  limits: Record<string, number | null>;
  ai_used: number;
  ai_reserved: number;
  demo_used: number;
  demo_reserved: number;
};
export type BillingData = {
  subscription: { status: string; region: string; trial_end: string | null };
  plan: { name: string; version: number };
  period: Period;
  resources: Record<string, number>;
  uncounted_files: number;
  periods: Period[];
  technical: {
    executions: number;
    input_tokens: number | null;
    output_tokens: number | null;
  };
};
export function Billing({ organizationId }: { organizationId: string }) {
  const { t, i18n } = useTranslation(),
    tr = (k: string) => t("billing." + k);
  const [data, setData] = useState<BillingData | null>(null),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const fmt = (s: string) =>
    new Intl.DateTimeFormat(i18n.resolvedLanguage, {
      dateStyle: "medium",
      timeStyle: "short",
      timeZone: "UTC",
    }).format(new Date(s));
  const num = (n: number) =>
    new Intl.NumberFormat(i18n.resolvedLanguage).format(n);
  async function load() {
    setBusy(true);
    setError("");
    try {
      setData(
        await api<BillingData>(`/organizations/${organizationId}/billing`),
      );
    } catch (e) {
      setError(e instanceof ApiError ? e.code : "internal");
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    void load();
  }, [organizationId]);
  function meter(key: string, used: number) {
    if (!data) return null;
    const cap = data.period.limits[key];
    const show = (n: number) =>
      key === "knowledge_bytes" ? num(n / 1048576) + " MiB" : num(n);
    return (
      <article className="billing-meter" key={key}>
        <div>
          <strong>{tr(key)}</strong>
          <span>
            {show(used)} / {cap === null ? tr("unlimited") : show(cap)}
          </span>
        </div>
        <progress
          aria-label={tr(key)}
          max={cap === null || cap === 0 ? 1 : cap}
          value={cap === null ? 0 : Math.min(used, cap || 1)}
        />
        {cap === 0 && <small>{tr("notAvailable")}</small>}
        {cap !== null && (
          <small>
            {tr("remaining")}: {show(Math.max(0, cap - used))}
          </small>
        )}
      </article>
    );
  }
  return (
    <section className="billing-page">
      {error && (
        <p className="alert" role="alert">
          {t("errors." + error, { defaultValue: t("errors.internal") })}
        </p>
      )}
      {!data && busy && <p role="status">{t("loading")}</p>}
      {data && (
        <>
          <section className="card billing-heading">
            <div>
              <span className="eyebrow">{tr("current")}</span>
              <h2>{data.plan.name}</h2>
              <p>
                {tr("status_" + data.subscription.status)} ·{" "}
                {data.subscription.region.toUpperCase()} · {tr("version")}{" "}
                {num(data.plan.version)}
              </p>
            </div>
            <button disabled={busy} onClick={() => void load()}>
              {tr("refresh")}
            </button>
            <p>
              {tr("period")}: {fmt(data.period.starts_at)} —{" "}
              {fmt(data.period.ends_at)} UTC
            </p>
            {data.subscription.status === "trialing" &&
              data.subscription.trial_end && (
                <p className="notice">
                  {tr("trialUntil")} {fmt(data.subscription.trial_end)} UTC
                </p>
              )}
            {["expired", "suspended"].includes(data.subscription.status) && (
              <p className="alert" role="status">
                {tr("inactiveHint")}
              </p>
            )}
            <p className="muted">{tr("pricePending")}</p>
          </section>
          <div className="billing-meters">
            {meter("ai_responses", data.period.ai_used)}
            {meter("demo_responses", data.period.demo_used)}
            {[
              "assistants",
              "sites",
              "members",
              "knowledge_sources",
              "knowledge_bytes",
            ].map((k) => meter(k, data.resources[k] || 0))}
          </div>
          <section className="card">
            <p>
              {tr("reserved")}: {num(data.period.ai_reserved)} ·{" "}
              {tr("demoReserved")}: {num(data.period.demo_reserved)}
            </p>
            <p>
              {tr("reservedStorage")}:{" "}
              {num((data.resources.reserved_bytes || 0) / 1048576)} MiB
            </p>
            {data.uncounted_files > 0 && (
              <p className="alert" role="status">
                {tr("storageIncomplete")}
              </p>
            )}
            <p className="muted">{tr("rules")}</p>
            <p className="muted">{tr("storageRule")}</p>
          </section>
          <section className="card">
            <h3>{tr("technical")}</h3>
            <p>
              {tr("executions")}: {num(data.technical.executions)}
            </p>
            <p>
              {tr("inputTokens")}:{" "}
              {data.technical.input_tokens === null
                ? tr("unknown")
                : num(data.technical.input_tokens)}{" "}
              · {tr("outputTokens")}:{" "}
              {data.technical.output_tokens === null
                ? tr("unknown")
                : num(data.technical.output_tokens)}
            </p>
            <p className="muted">{tr("tokenRule")}</p>
          </section>
          <section className="card">
            <h3>{tr("history")}</h3>
            {data.periods.map((p) => (
              <article className="billing-history" key={p.id}>
                <span>
                  {fmt(p.starts_at)} — {fmt(p.ends_at)} UTC
                </span>
                <strong>
                  {tr("ai_responses")}: {num(p.ai_used)} ·{" "}
                  {tr("demo_responses")}: {num(p.demo_used)}
                </strong>
              </article>
            ))}
          </section>
        </>
      )}
    </section>
  );
}
