import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "./api";
type Invite = {
  id: string;
  role: string;
  url?: string;
  expires_at: string;
  revoked_at: string | null;
  accepted_at: string | null;
};
type Member = { id: string; name: string; role: string };
export function Team({ organizationId }: { organizationId: string }) {
  const { t, i18n } = useTranslation();
  const tr = (k: string) => t("inbox." + k);
  const [items, setItems] = useState<Invite[]>([]),
    [members, setMembers] = useState<Member[]>([]),
    [role, setRole] = useState("operator"),
    [url, setUrl] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const base = "/organizations/" + organizationId;
  async function load() {
    const [i, m] = await Promise.all([
      api<Invite[]>(base + "/invitations"),
      api<Member[]>(base + "/members"),
    ]);
    setItems(i);
    setMembers(m);
  }
  function fail(e: unknown) {
    setError(e instanceof ApiError ? e.code : "network");
  }
  useEffect(() => {
    void load().catch(fail);
  }, [base]);
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
    <section className="card team-panel">
      <h2>{tr("team")}</h2>
      {error && (
        <div className="alert" role="alert">
          {t("errors." + error, { defaultValue: t("errors.internal") })}
        </div>
      )}
      <div className="team-members">
        {members.map((m) => (
          <div key={m.id}>
            <strong>{m.name || tr("staff") + " #" + m.id.slice(0, 6)}</strong>
            <span>{t("roles." + m.role)}</span>
          </div>
        ))}
      </div>
      <h3>{tr("invite")}</h3>
      <p>{tr("inviteHint")}</p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void act(async () => {
            const v = await api<Invite>(base + "/invitations", "POST", {
              role,
            });
            setUrl(v.url || "");
          });
        }}
      >
        <label>
          {tr("role")}
          <select value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="operator">{t("roles.operator")}</option>
            <option value="admin">{t("roles.admin")}</option>
          </select>
        </label>
        <button className="primary" disabled={busy}>
          {tr("createInvite")}
        </button>
      </form>
      {url && (
        <label>
          {tr("invitationLink")}
          <input
            aria-label={tr("invitationLink")}
            readOnly
            value={url}
            onFocus={(e) => e.target.select()}
          />
          <button
            onClick={() =>
              void navigator.clipboard.writeText(url).catch(() => {})
            }
          >
            {tr("copy")}
          </button>
        </label>
      )}
      <h3>{tr("invitations")}</h3>
      {items.map((v) => (
        <div key={v.id} className="invitation-row">
          <span>
            {t("roles." + v.role)} ·{" "}
            {new Intl.DateTimeFormat(i18n.resolvedLanguage, {
              dateStyle: "short",
            }).format(new Date(v.expires_at))}{" "}
            ·{" "}
            {tr(
              v.revoked_at
                ? "revoked"
                : v.accepted_at
                  ? "accepted"
                  : new Date(v.expires_at) < new Date()
                    ? "expired"
                    : "pending",
            )}
          </span>
          {!v.revoked_at && !v.accepted_at && (
            <button
              disabled={busy}
              onClick={() =>
                void act(async () => {
                  await api(base + "/invitations/" + v.id, "DELETE", {});
                })
              }
            >
              {tr("revoke")}
            </button>
          )}
        </div>
      ))}
    </section>
  );
}
