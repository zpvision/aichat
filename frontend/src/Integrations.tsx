import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "./api";
type Key = {
  id: string;
  name: string;
  scopes: string[];
  expires_at: string;
  revoked_at: string | null;
  last_used_at: string | null;
};
type Endpoint = { id: string; url: string; enabled: boolean };
type Delivery = {
  id: string;
  endpoint_id: string;
  event_id: string;
  status: string;
  attempts: number;
  last_code: string;
  http_status: number;
  created_at: string;
};
type Attempt = {
  id: string;
  attempt: number;
  code: string;
  http_status: number;
  duration_ms: number;
  created_at: string;
};
export function Integrations({ organizationId }: { organizationId: string }) {
  const { t, i18n } = useTranslation(),
    tr = (k: string) => t("integration." + k),
    base = `/organizations/${organizationId}/integrations`;
  const [keys, setKeys] = useState<Key[]>([]),
    [endpoints, setEndpoints] = useState<Endpoint[]>([]),
    [deliveries, setDeliveries] = useState<Delivery[]>([]),
    [older, setOlder] = useState(false),
    [attempts, setAttempts] = useState<Attempt[]>([]),
    [selected, setSelected] = useState(""),
    [secret, setSecret] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [name, setName] = useState(""),
    [days, setDays] = useState(90),
    [write, setWrite] = useState(false),
    [url, setURL] = useState("");
  const fmt = (v: string) =>
    new Intl.DateTimeFormat(i18n.resolvedLanguage, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(v));
  function fail(e: unknown) {
    setError(e instanceof ApiError ? e.code : "internal");
  }
  async function load() {
    const [k, e, d] = await Promise.all([
      api<Key[]>(base + "/keys"),
      api<Endpoint[]>(base + "/webhooks"),
      api<Delivery[]>(base + "/deliveries"),
    ]);
    setKeys(k);
    setEndpoints(e);
    if (!older) setDeliveries(d);
  }
  useEffect(() => {
    let active = true,
      running = false;
    const poll = async () => {
      if (document.hidden || running || older) return;
      running = true;
      try {
        const d = await api<Delivery[]>(base + "/deliveries");
        if (active) setDeliveries(d);
        if (selected) {
          const a = await api<Attempt[]>(
            base + "/deliveries/" + selected + "/attempts",
          );
          if (active) setAttempts(a);
        }
      } catch (e) {
        if (active) fail(e);
      } finally {
        running = false;
      }
    };
    if (!older) void load().catch(fail);
    const timer = setInterval(() => void poll(), 3000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [base, selected, older]);
  async function act(fn: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await fn();
      await load();
    } catch (e) {
      fail(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="integration-page">
      {error && (
        <p className="alert" role="alert">
          {t("errors." + error, { defaultValue: t("errors.internal") })}
        </p>
      )}
      {secret && (
        <section className="card integration-secret" role="status">
          <h2>{tr("secretOnce")}</h2>
          <p>{tr("secretHint")}</p>
          <textarea
            aria-label={tr("secret")}
            value={secret}
            readOnly
            rows={3}
          />
          <button onClick={() => setSecret("")}>{tr("hideSecret")}</button>
        </section>
      )}
      <div className="integration-grid">
        <section className="card">
          <h2>{tr("keys")}</h2>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void act(async () => {
                const k = await api<{ token: string }>(base + "/keys", "POST", {
                  name,
                  days,
                  scopes: write
                    ? ["leads:read", "leads:write"]
                    : ["leads:read"],
                });
                setSecret(k.token);
                setName("");
              });
            }}
          >
            <label>
              {tr("name")}
              <input
                required
                maxLength={100}
                value={name}
                disabled={busy}
                onChange={(e) => setName(e.target.value)}
              />
            </label>
            <label>
              {tr("expiry")}
              <input
                type="number"
                min={1}
                max={365}
                value={days}
                disabled={busy}
                onChange={(e) => setDays(Number(e.target.value))}
              />
            </label>
            <label className="check">
              <input
                type="checkbox"
                checked={write}
                disabled={busy}
                onChange={(e) => setWrite(e.target.checked)}
              />
              {tr("allowWrite")}
            </label>
            <button className="primary" disabled={busy || !name.trim()}>
              {tr("createKey")}
            </button>
          </form>
          <div className="integration-items">
            {keys.length === 0 && <p>{tr("emptyKeys")}</p>}
            {keys.map((k) => (
              <article key={k.id}>
                <strong>{k.name}</strong>
                <p>{k.scopes.join(", ")}</p>
                <small>
                  {tr("expires")}: {fmt(k.expires_at)}
                </small>
                {k.last_used_at && (
                  <small>
                    {tr("lastUsed")}: {fmt(k.last_used_at)}
                  </small>
                )}
                {k.revoked_at ? (
                  <p>{tr("revoked")}</p>
                ) : (
                  <button
                    className="danger"
                    disabled={busy}
                    onClick={() => {
                      if (window.confirm(tr("revokeConfirm")))
                        void act(async () => {
                          await api(base + "/keys/" + k.id, "DELETE");
                        });
                    }}
                  >
                    {tr("revoke")}
                  </button>
                )}
              </article>
            ))}
          </div>
        </section>
        <section className="card">
          <h2>{tr("webhooks")}</h2>
          <p>{tr("metadataOnly")}</p>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void act(async () => {
                const v = await api<{ secret: string }>(
                  base + "/webhooks",
                  "POST",
                  { url },
                );
                setSecret(v.secret);
                setURL("");
              });
            }}
          >
            <label>
              {tr("url")}
              <input
                type="url"
                required
                maxLength={2000}
                value={url}
                disabled={busy}
                onChange={(e) => setURL(e.target.value)}
              />
            </label>
            <button className="primary" disabled={busy || !url}>
              {tr("createWebhook")}
            </button>
          </form>
          <div className="integration-items">
            {endpoints.length === 0 && <p>{tr("emptyWebhooks")}</p>}
            {endpoints.map((e) => (
              <article key={e.id}>
                <strong>{e.url}</strong>
                <p>{tr(e.enabled ? "enabled" : "disabled")}</p>
                <div className="lead-actions">
                  <button
                    disabled={busy}
                    onClick={() =>
                      void act(async () => {
                        await api(base + "/webhooks/" + e.id, "PATCH", {
                          enabled: !e.enabled,
                        });
                      })
                    }
                  >
                    {tr(e.enabled ? "disable" : "enable")}
                  </button>
                  <button
                    disabled={busy || !e.enabled}
                    onClick={() =>
                      void act(async () => {
                        await api(
                          base + "/webhooks/" + e.id + "/test",
                          "POST",
                          {},
                        );
                      })
                    }
                  >
                    {tr("test")}
                  </button>
                  <button
                    disabled={busy}
                    onClick={() => {
                      if (window.confirm(tr("rotateConfirm")))
                        void act(async () => {
                          const v = await api<{ secret: string }>(
                            base + "/webhooks/" + e.id + "/rotate",
                            "POST",
                            {},
                          );
                          setSecret(v.secret);
                        });
                    }}
                  >
                    {tr("rotate")}
                  </button>
                </div>
              </article>
            ))}
          </div>
        </section>
      </div>
      <section className="card">
        <div className="page-heading">
          <h2>{tr("deliveries")}</h2>
          <button disabled={busy} onClick={() => void act(async () => {})}>
            {tr("refresh")}
          </button>
        </div>
        <p>{tr("historyLimit")}</p>
        {deliveries.length === 0 && <p>{tr("emptyDeliveries")}</p>}
        <div className="lead-actions">
          {older && (
            <button
              onClick={() =>
                void act(async () => {
                  setOlder(false);
                })
              }
            >
              {tr("refresh")}
            </button>
          )}
          {deliveries.length === 100 && (
            <button
              disabled={busy}
              onClick={() => {
                setBusy(true);
                setOlder(true);
                void api<Delivery[]>(
                  base + "/deliveries?before=" + deliveries.at(-1)!.id,
                )
                  .then(setDeliveries)
                  .catch(fail)
                  .finally(() => setBusy(false));
              }}
            >
              {t("lead.more")}
            </button>
          )}
        </div>
        <div className="integration-deliveries">
          {deliveries.map((d) => (
            <article key={d.id}>
              <strong>{tr("state_" + d.status)}</strong>
              <small>{fmt(d.created_at)}</small>
              <code>{d.event_id}</code>
              <p>
                {tr("attempts")}: {d.attempts} · HTTP {d.http_status || "—"}
              </p>
              <p>
                {tr("result")}: {tr("code_" + d.last_code)}
              </p>
              <div className="lead-actions">
                <button
                  onClick={() =>
                    void act(async () => {
                      setSelected(d.id);
                      setAttempts(
                        await api<Attempt[]>(
                          base + "/deliveries/" + d.id + "/attempts",
                        ),
                      );
                    })
                  }
                >
                  {tr("journal")}
                </button>
                {["failed", "canceled"].includes(d.status) && (
                  <button
                    disabled={busy}
                    onClick={() =>
                      void act(async () => {
                        await api(
                          base + "/deliveries/" + d.id + "/retry",
                          "POST",
                          {},
                        );
                      })
                    }
                  >
                    {tr("retry")}
                  </button>
                )}
              </div>
            </article>
          ))}
        </div>
        {selected && (
          <section aria-label={tr("journal")} className="integration-journal">
            <h3>{tr("journal")}</h3>
            {attempts.map((a) => (
              <p key={a.id}>
                #{a.attempt} · {tr("code_" + a.code)} · HTTP{" "}
                {a.http_status || "—"} ·{" "}
                {new Intl.NumberFormat(i18n.resolvedLanguage).format(
                  a.duration_ms,
                )}{" "}
                ms · {fmt(a.created_at)}
              </p>
            ))}
          </section>
        )}
      </section>
    </section>
  );
}
