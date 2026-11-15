import { useEffect, useId, useState, useRef } from "react";
import { useTranslation } from "react-i18next";
import i18n from "../i18n";
import { api, type Assistant, type Site } from "../api";
import type { BillingData } from "../Billing";
import en from "../locales/en/analytics.json";
import ru from "../locales/ru/analytics.json";
import cs from "../locales/cs/analytics.json";
import de from "../locales/de/analytics.json";
import "./analytics.css";
for (const [language, copy] of Object.entries({ en, ru, cs, de }))
  i18n.addResourceBundle(language, "analytics", copy, true, true);
type Copy = typeof en;
type Kpi = {
  dialogs: number;
  leads: number;
  eligible: number;
  converted: number;
  without_operator: number;
  handoffs: number;
  generation_ms: number | null;
};
type Point = {
  bucket_date: string;
  dialogs: number;
  leads: number;
  handoffs: number;
  no_sources: number;
  failed: number;
  canceled: number;
};
type Row = {
  site_id: string;
  assistant_id: string;
  site: string;
  assistant: string;
  dialogs: number;
  leads: number;
  handoffs: number;
  conversion: number | null;
};
type Report = {
  period: { start: string; end: string; timezone: string };
  kpis: { current: Kpi; previous: Kpi };
  timeseries: Point[];
  handling: {
    without_operator: number;
    with_operator: number;
    waiting_now: number;
    human_active_now: number;
  };
  quality: { no_sources: number; failed: number; canceled: number };
  executions: {
    provider: string;
    status: keyof Copy["statuses"];
    count: number;
    input_tokens: number | null;
    output_tokens: number | null;
  }[];
  lead_sources: {
    source: keyof Copy["sourceNames"];
    status: keyof Copy["statuses"];
    count: number;
  }[];
  sites: Row[];
  sites_total: number;
  insights: {
    top_site: { site: string; leads: number } | null;
    top_assistant: { assistant: string; handoffs: number } | null;
  };
  attention: {
    id: string;
    conversation_id: string;
    site: string;
    assistant: string;
    category: keyof Copy["categories"];
    created_at: string;
  }[];
  attention_total: number;
  operators: {
    user_id: string;
    name: string;
    taken: number;
    replies: number;
    closed: number;
    assigned_now: number;
    response_ms: number | null;
    response_samples: number;
  }[];
  operators_total: number;
};
type Props = {
  organizationId: string;
  timezone: string;
  sites: Site[];
  assistants: Assistant[];
  onConversation: (id: string) => void;
  onLeads: () => void;
  onBilling: () => void;
};
type Filters = {
  from: string;
  to: string;
  timezone: string;
  site: string;
  assistant: string;
};
function dateInZone(date: Date, zone: string) {
  const p = new Intl.DateTimeFormat("en-CA", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  return ["year", "month", "day"]
    .map((k) => p.find((x) => x.type === k)?.value)
    .join("-");
}
function shift(s: string, days: number) {
  const d = new Date(s + "T12:00:00Z");
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}
export function ratio(n: number, d: number) {
  return d > 0 ? (n / d) * 100 : null;
}
export function change(now: number | null, previous: number | null) {
  return now !== null && previous !== null && previous !== 0
    ? ((now - previous) / Math.abs(previous)) * 100
    : null;
}
function values(k: Kpi) {
  return [
    k.dialogs,
    k.leads,
    ratio(k.converted, k.eligible),
    ratio(k.without_operator, k.dialogs),
    k.handoffs,
    k.generation_ms,
  ];
}
function Chart({
  points,
  d,
  locale,
}: {
  points: Point[];
  d: Copy;
  locale: string;
}) {
  const [visible, setVisible] = useState([true, true, true]),
    [selected, setSelected] = useState<number | null>(null);
  const id = useId();
  const plot = useRef<HTMLDivElement>(null),
    [plotWidth, setPlotWidth] = useState(930);
  useEffect(() => {
    if (!plot.current || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver((entries) =>
      setPlotWidth(Math.max(250, Math.round(entries[0].contentRect.width))),
    );
    observer.observe(plot.current);
    return () => observer.disconnect();
  }, []);
  const keys = ["dialogs", "leads", "handoffs"] as const,
    colors = ["#5530c8", "#b55fb2", "#5d8098"];
  const max =
    Math.ceil(
      Math.max(
        4,
        ...points.flatMap((p) => keys.map((k, i) => (visible[i] ? p[k] : 0))),
      ) / 4,
    ) * 4;
  const x = (i: number) =>
      points.length === 1
        ? plotWidth / 2
        : 45 + i * ((plotWidth - 65) / Math.max(1, points.length - 1)),
    y = (n: number) => 246 - (n / max) * 210;
  const day = (s: string) =>
    new Intl.DateTimeFormat(locale, {
      day: "numeric",
      month: "short",
      timeZone: "UTC",
    }).format(new Date(s + "T12:00:00Z"));
  return (
    <div className="an-chart">
      <div className="an-chart-legend">
        {d.series.map((s, i) => (
          <button
            key={s}
            aria-pressed={visible[i]}
            onClick={() =>
              setVisible((v) => v.map((x, j) => (j === i ? !x : x)))
            }
          >
            <i style={{ background: colors[i] }} />
            {s}
          </button>
        ))}
      </div>
      <div className="an-chart-scroll" ref={plot}>
        <svg viewBox={`0 0 ${plotWidth} 290`} role="img" aria-label={d.trend}>
          <defs>
            <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
              <stop stopColor="#9957e8" stopOpacity=".18" />
              <stop offset="1" stopColor="#9957e8" stopOpacity="0" />
            </linearGradient>
          </defs>
          {[0, 0.25, 0.5, 0.75, 1].map((v) => (
            <g key={v}>
              <path
                d={`M45 ${y(v * max)}H${plotWidth - 20}`}
                stroke="#e8e2f2"
                strokeDasharray="4 5"
              />
              <text x="40" y={y(v * max) + 4} textAnchor="end">
                {Math.round(v * max)}
              </text>
            </g>
          ))}
          {keys.map(
            (k, j) =>
              visible[j] && (
                <g key={k}>
                  {j === 0 && points.length > 0 && (
                    <path
                      d={`M${x(0)} 246 ${points.map((p, i) => `L${x(i)} ${y(p[k])}`).join(" ")} L${x(points.length - 1)} 246Z`}
                      fill={`url(#${id})`}
                    />
                  )}
                  <polyline
                    points={points
                      .map((p, i) => `${x(i)},${y(p[k])}`)
                      .join(" ")}
                    stroke={colors[j]}
                    fill="none"
                    strokeWidth="3"
                    strokeLinejoin="round"
                  />
                  {points.map((p, i) => (
                    <circle
                      key={i}
                      cx={x(i)}
                      cy={y(p[k])}
                      r={selected === i ? 5 : 2.5}
                      fill={colors[j]}
                    />
                  ))}
                </g>
              ),
          )}
          {points.map((p, i) => (
            <g key={p.bucket_date}>
              {(i %
                Math.max(
                  1,
                  Math.ceil(points.length / (plotWidth < 500 ? 3 : 7)),
                ) ===
                0 ||
                i === points.length - 1) && (
                <text
                  x={x(i)}
                  y="279"
                  textAnchor={
                    i === 0
                      ? "start"
                      : i === points.length - 1
                        ? "end"
                        : "middle"
                  }
                >
                  {day(p.bucket_date)}
                </text>
              )}
              <rect
                x={x(i) - (plotWidth - 65) / 2 / Math.max(1, points.length - 1)}
                y="20"
                width={(plotWidth - 65) / Math.max(1, points.length - 1)}
                height="240"
                fill="transparent"
                onMouseEnter={() => setSelected(i)}
                onClick={() => setSelected(i)}
              >
                <title>
                  {day(p.bucket_date) +
                    ": " +
                    keys.map((k, j) => `${d.series[j]} ${p[k]}`).join(", ")}
                </title>
              </rect>
            </g>
          ))}
        </svg>
      </div>
      <label className="an-chart-point">
        {d.range}
        <select
          value={selected ?? ""}
          onChange={(e) =>
            setSelected(e.target.value === "" ? null : Number(e.target.value))
          }
        >
          <option value="">{d.all}</option>
          {points.map((p, i) => (
            <option key={p.bucket_date} value={i}>
              {day(p.bucket_date)}
            </option>
          ))}
        </select>
        {selected !== null && points[selected] && (
          <span>
            {keys
              .map((k, i) => `${d.series[i]}: ${points[selected][k]}`)
              .join(" · ")}
          </span>
        )}
      </label>
    </div>
  );
}
export function Analytics({
  organizationId,
  timezone,
  sites,
  assistants,
  onConversation,
  onLeads,
  onBilling,
}: Props) {
  const { i18n } = useTranslation("analytics");
  // Namespace root is accessed explicitly to avoid mixing cabinet dictionaries.
  const copy =
    (i18n.getResourceBundle(
      i18n.resolvedLanguage || "en",
      "analytics",
    ) as Copy) || en;
  const c = copy;
  const locale = i18n.resolvedLanguage || "en";
  const number = new Intl.NumberFormat(locale, { maximumFractionDigits: 1 });
  const initial = () => {
    const q = new URL(location.href).searchParams,
      today = dateInZone(new Date(), timezone);
    return {
      from: q.get("an_from") || shift(today, -6),
      to: q.get("an_to") || today,
      timezone: [
        timezone,
        "UTC",
        "Europe/Prague",
        "Europe/Berlin",
        "Europe/Moscow",
      ].includes(q.get("an_timezone") || "")
        ? q.get("an_timezone")!
        : timezone,
      site: sites.some((s) => s.id === q.get("an_site"))
        ? q.get("an_site")!
        : "",
      assistant: assistants.some((a) => a.id === q.get("an_assistant"))
        ? q.get("an_assistant")!
        : "",
    };
  };
  const [filters, setFilters] = useState<Filters>(initial),
    [draft, setDraft] = useState<Filters>(initial),
    [tab, setTab] = useState(0),
    [page, setPage] = useState(0),
    [sort, setSort] = useState("dialogs"),
    [data, setData] = useState<Report | null>(null),
    [busy, setBusy] = useState(true),
    [error, setError] = useState(false),
    [retry, setRetry] = useState(0);
  useEffect(() => {
    let active = true;
    setBusy(true);
    setError(false);
    const q = new URLSearchParams({ ...filters, page: String(page), sort });
    const url = new URL(location.href);
    for (const [k, v] of Object.entries(filters))
      url.searchParams.set("an_" + k, v);
    history.replaceState(null, "", url);
    api<Report>(`/organizations/${organizationId}/analytics?${q}`)
      .then((v) => {
        if (active) setData(v);
      })
      .catch(() => {
        if (active) {
          setError(true);
          setData(null);
        }
      })
      .finally(() => {
        if (active) setBusy(false);
      });
    return () => {
      active = false;
    };
  }, [organizationId, filters, page, sort, retry]);
  const format = (v: number | null, i = 0) =>
    v === null
      ? c.missing
      : i === 2 || i === 3
        ? number.format(v) + "%"
        : i === 5
          ? new Intl.NumberFormat(locale, {
              style: "unit",
              unit: "second",
              unitDisplay: "short",
              maximumFractionDigits: 2,
            }).format(v / 1000)
          : number.format(v);
  const [billing, setBilling] = useState<BillingData | null>(null);
  useEffect(() => {
    let alive = true;
    api<BillingData>(`/organizations/${organizationId}/billing`)
      .then((v) => {
        if (alive) setBilling(v);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [organizationId, retry]);
  const current = data ? values(data.kpis.current) : [],
    previous = data ? values(data.kpis.previous) : [];
  function preset(index: number) {
    if (index === 4) return;
    const today = dateInZone(new Date(), draft.timezone),
      to = index === 1 ? shift(today, -1) : today;
    const f = {
      ...draft,
      from: index < 2 ? to : shift(to, index === 2 ? -6 : -29),
      to,
    };
    setDraft(f);
    setFilters(f);
    setPage(0);
  }
  const topLeads = data?.insights.top_site,
    topHandoff = data?.insights.top_assistant;
  const table = (headers: string[], rows: React.ReactNode[][]) => (
    <div
      className="an-table-scroll"
      tabIndex={0}
      role="region"
      aria-label={headers.join(" · ")}
    >
      <table>
        <thead>
          <tr>
            {headers.map((h, i) => (
              <th key={i}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              {r.map((v, j) => (
                <td key={j}>{v}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
  const total =
    tab === 1
      ? data?.attention_total
      : tab === 2
        ? data?.operators_total
        : data?.sites_total;
  return (
    <section className="analytics" aria-busy={busy}>
      <header className="an-intro">
        <div>
          <span className="an-eyebrow">AI CHAT / {c.nav}</span>
          <h2>{c.title}</h2>
          <p>{c.subtitle}</p>
        </div>
        <span className="an-scope">{c.scope}</span>
      </header>
      <div className="an-tabs" role="tablist" aria-label={c.nav}>
        {c.tabs.map((v, i) => (
          <button
            key={v}
            role="tab"
            id={"an-tab-" + i}
            aria-controls="an-panel"
            aria-selected={tab === i}
            tabIndex={tab === i ? 0 : -1}
            onKeyDown={(e) => {
              if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(e.key))
                return;
              e.preventDefault();
              const next =
                e.key === "Home"
                  ? 0
                  : e.key === "End"
                    ? 2
                    : (i + (e.key === "ArrowRight" ? 1 : 2)) % 3;
              setTab(next);
              setPage(0);
              (
                e.currentTarget.parentElement?.querySelectorAll("button")[
                  next
                ] as HTMLButtonElement
              )?.focus();
            }}
            onClick={() => {
              setTab(i);
              setPage(0);
            }}
          >
            {v}
          </button>
        ))}
      </div>
      <form
        className="an-filters"
        onSubmit={(e) => {
          e.preventDefault();
          setFilters(draft);
          setPage(0);
        }}
      >
        <div className="an-presets">
          {c.filters.slice(0, 4).map((v, i) => (
            <button type="button" key={v} onClick={() => preset(i)}>
              {v}
            </button>
          ))}
        </div>
        <div className="an-filter-fields">
          <label>
            {c.from}
            <input
              type="date"
              required
              value={draft.from}
              onChange={(e) => setDraft({ ...draft, from: e.target.value })}
            />
          </label>
          <label>
            {c.to}
            <input
              type="date"
              required
              min={draft.from}
              value={draft.to}
              onChange={(e) => setDraft({ ...draft, to: e.target.value })}
            />
          </label>
          <label>
            {c.zone}
            <select
              value={draft.timezone}
              onChange={(e) => setDraft({ ...draft, timezone: e.target.value })}
            >
              {[
                ...new Set([
                  timezone,
                  "UTC",
                  "Europe/Prague",
                  "Europe/Berlin",
                  "Europe/Moscow",
                ]),
              ].map((z) => (
                <option key={z}>{z}</option>
              ))}
            </select>
          </label>
          <label>
            {c.site}
            <select
              value={draft.site}
              onChange={(e) => setDraft({ ...draft, site: e.target.value })}
            >
              <option value="">{c.all}</option>
              {sites.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            {c.assistant}
            <select
              value={draft.assistant}
              onChange={(e) =>
                setDraft({ ...draft, assistant: e.target.value })
              }
            >
              <option value="">{c.all}</option>
              {assistants.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </label>
          <button className="an-primary">{c.apply}</button>
        </div>
      </form>
      {error && (
        <div role="alert" className="an-error">
          {c.error}
          <button onClick={() => setRetry((x) => x + 1)}>{c.retry}</button>
        </div>
      )}
      {busy && (
        <div className="an-loading" role="status">
          {c.loading}
        </div>
      )}
      <div
        className={busy ? "an-content an-updating" : "an-content"}
        id="an-panel"
        role="tabpanel"
        aria-labelledby={"an-tab-" + tab}
      >
        {!data ? (
          busy ? (
            <div className="an-skeleton" />
          ) : null
        ) : (
          <>
            <div className="an-kpis">
              {c.kpis.map((title, i) => {
                const delta = change(current[i], previous[i]);
                return (
                  <article key={title} className={"an-kpi an-kpi-" + i}>
                    <div>
                      <span>{title}</span>
                      <details>
                        <summary aria-label={title + " — " + c.range}>
                          i
                        </summary>
                        <p>{c.definitions[i]}</p>
                      </details>
                    </div>
                    <strong>{format(current[i], i)}</strong>
                    <small>
                      {delta === null
                        ? c.noBase
                        : (delta > 0 ? "+" : "") +
                          number.format(delta) +
                          "% " +
                          c.previous}
                    </small>
                  </article>
                );
              })}
            </div>
            {data.kpis.current.dialogs === 0 &&
              data.lead_sources.length === 0 &&
              data.executions.length === 0 &&
              data.kpis.current.handoffs === 0 &&
              !data.operators.some((o) => o.replies || o.taken || o.closed) && (
                <div className="an-empty">
                  <span>◌</span>
                  <h3>{c.empty}</h3>
                  <p>{c.emptyBody}</p>
                </div>
              )}
            {tab === 0 && (
              <>
                <article className="an-panel an-trend">
                  <div className="an-panel-heading">
                    <div>
                      <span className="an-eyebrow">01 / {c.range}</span>
                      <h3>{c.trend}</h3>
                    </div>
                    <span>
                      {dateInZone(
                        new Date(data.period.start),
                        data.period.timezone,
                      )}{" "}
                      —{" "}
                      {dateInZone(
                        new Date(new Date(data.period.end).getTime() - 1),
                        data.period.timezone,
                      )}{" "}
                      · {data.period.timezone}
                    </span>
                  </div>
                  <Chart points={data.timeseries} d={c} locale={locale} />
                </article>
                <div className="an-two">
                  <article className="an-panel an-handling">
                    <h3>{c.handling}</h3>
                    <div className="an-handling-total">
                      <strong>
                        {number.format(data.kpis.current.dialogs)}
                      </strong>
                      <span>{c.kpis[0]}</span>
                    </div>
                    <div className="an-stacked">
                      <i
                        style={{
                          width:
                            (ratio(
                              data.handling.without_operator,
                              data.kpis.current.dialogs,
                            ) || 0) + "%",
                        }}
                      />
                      <i
                        style={{
                          display:
                            data.kpis.current.dialogs > 0 ? "block" : "none",
                        }}
                      />
                    </div>
                    {[
                      [c.without, data.handling.without_operator],
                      [c.with, data.handling.with_operator],
                    ].map(([label, n]) => (
                      <p key={label}>
                        <span>{label}</span>
                        <b>{number.format(Number(n))}</b>
                      </p>
                    ))}
                    <small>{c.noPrediction}</small>
                    <div className="an-now">
                      <span>
                        {c.waiting}
                        <b>{data.handling.waiting_now}</b>
                      </span>
                      <span>
                        {c.active}
                        <b>{data.handling.human_active_now}</b>
                      </span>
                    </div>
                  </article>
                  <article className="an-panel an-insights">
                    <span className="an-eyebrow">AI CHAT / {c.range}</span>
                    <h3>{c.insights}</h3>
                    {data.kpis.current.dialogs > 0 ||
                    topLeads ||
                    topHandoff ||
                    data.quality.failed > 0 ||
                    change(current[0], previous[0]) !== null ? (
                      <div className="an-observations">
                        {change(current[0], previous[0]) !== null && (
                          <p>
                            <span>{c.insightGrowth}</span>
                            <strong>
                              {number.format(change(current[0], previous[0])!)}%
                            </strong>
                          </p>
                        )}
                        {topLeads && (
                          <p>
                            <span>{c.insightLeads}</span>
                            <strong>
                              {topLeads.site} · {topLeads.leads}
                            </strong>
                          </p>
                        )}
                        {topHandoff && (
                          <p>
                            <span>{c.insightHandoff}</span>
                            <strong>
                              {topHandoff.assistant} · {topHandoff.handoffs}
                            </strong>
                          </p>
                        )}
                        {data.quality.failed > 0 && (
                          <p>
                            <span>{c.insightErrors}</span>
                            <strong>{data.quality.failed}</strong>
                          </p>
                        )}
                        {!topLeads &&
                          !topHandoff &&
                          !data.quality.failed &&
                          change(current[0], previous[0]) === null && (
                            <p>{c.noBase}</p>
                          )}
                      </div>
                    ) : (
                      <p>{c.emptyBody}</p>
                    )}
                  </article>
                </div>
                <article className="an-panel">
                  <div className="an-panel-heading">
                    <h3>{c.sources}</h3>
                    <button onClick={onLeads}>{c.leads} ↗</button>
                  </div>
                  {table(
                    [c.source, c.status, c.kpis[1]],
                    data.lead_sources.map((x) => [
                      c.sourceNames[x.source],
                      c.statuses[x.status],
                      number.format(x.count),
                    ]),
                  )}
                </article>
                <article className="an-panel">
                  <div className="an-panel-heading">
                    <h3>{c.breakdown}</h3>
                    <label>
                      {c.sort}
                      <select
                        value={sort}
                        onChange={(e) => {
                          setSort(e.target.value);
                          setPage(0);
                        }}
                      >
                        {["dialogs", "leads", "handoffs", "conversion"].map(
                          (v, i) => (
                            <option key={v} value={v}>
                              {c.kpis[[0, 1, 4, 2][i]]}
                            </option>
                          ),
                        )}
                      </select>
                    </label>
                  </div>
                  {table(
                    [
                      c.site,
                      c.assistant,
                      c.kpis[0],
                      c.kpis[1],
                      c.kpis[2],
                      c.kpis[4],
                    ],
                    data.sites.map((x) => [
                      x.site,
                      x.assistant,
                      number.format(x.dialogs),
                      number.format(x.leads),
                      format(
                        x.conversion === null ? null : x.conversion * 100,
                        2,
                      ),
                      number.format(x.handoffs),
                    ]),
                  )}
                </article>
              </>
            )}
            {tab === 1 && (
              <>
                <article className="an-panel">
                  <span className="an-eyebrow">02 / {c.tabs[1]}</span>
                  <h3>{c.quality}</h3>
                  <p className="an-note">{c.qualityNote}</p>
                  <div className="an-quality-counts">
                    {[
                      [c.categories.no_sources, data.quality.no_sources],
                      [c.statuses.failed, data.quality.failed],
                      [c.statuses.canceled, data.quality.canceled],
                    ].map(([label, n]) => (
                      <div key={label}>
                        <strong>{n}</strong>
                        <span>{label}</span>
                      </div>
                    ))}
                  </div>
                  <Chart
                    points={data.timeseries.map((p) => ({
                      ...p,
                      dialogs: p.no_sources,
                      leads: p.failed,
                      handoffs: p.canceled,
                    }))}
                    d={{
                      ...c,
                      trend: c.quality,
                      series: [
                        c.categories.no_sources,
                        c.statuses.failed,
                        c.statuses.canceled,
                      ],
                    }}
                    locale={locale}
                  />
                  {table(
                    [c.range, c.site, c.assistant, c.tabs[1], c.open],
                    data.attention.map((x) => [
                      new Date(x.created_at).toLocaleString(locale, {
                        timeZone: data.period.timezone,
                      }),
                      x.site,
                      x.assistant,
                      c.categories[x.category],
                      <button onClick={() => onConversation(x.conversation_id)}>
                        {c.open} ↗
                      </button>,
                    ]),
                  )}
                </article>
                <article className="an-panel">
                  <h3>{c.execution}</h3>
                  {table(
                    [c.provider, c.status, c.executionCount, c.tokens],
                    data.executions.map((x) => [
                      x.provider === "mock" ? "Mock · " + c.demo : x.provider,
                      c.statuses[x.status],
                      number.format(x.count),
                      (x.input_tokens === null
                        ? c.missing
                        : number.format(x.input_tokens)) +
                        " / " +
                        (x.output_tokens === null
                          ? c.missing
                          : number.format(x.output_tokens)),
                    ]),
                  )}
                </article>
              </>
            )}
            {tab === 2 && (
              <article className="an-panel">
                <span className="an-eyebrow">03 / {c.tabs[2]}</span>
                <h3>{c.team}</h3>
                <p className="an-note">{c.teamNote}</p>
                {table(
                  c.columns,
                  data.operators.map((x) => [
                    x.name || c.noNames,
                    number.format(x.taken),
                    number.format(x.replies),
                    number.format(x.closed),
                    number.format(x.assigned_now),
                    format(x.response_ms, 5),
                    number.format(x.response_samples),
                  ]),
                )}
              </article>
            )}
            <div className="an-pagination">
              <span>
                {number.format(total || 0)} {c.rows}
              </span>
              <button
                disabled={page === 0}
                onClick={() => setPage((n) => n - 1)}
              >
                {c.back}
              </button>
              <span>{page + 1}</span>
              <button
                disabled={(page + 1) * 20 >= (total || 0)}
                onClick={() => setPage((n) => n + 1)}
              >
                {c.next}
              </button>
            </div>
            <article className="an-usage">
              <div>
                <h3>{c.usage}</h3>
                <p>{c.usageNote}</p>
                {billing ? (
                  <div className="an-live-usage">
                    {(["ai", "demo"] as const).map((kind) => {
                      const used =
                        kind === "ai"
                          ? billing.period.ai_used
                          : billing.period.demo_used;
                      const reserved =
                        kind === "ai"
                          ? billing.period.ai_reserved
                          : billing.period.demo_reserved;
                      const cap =
                        billing.period.limits[
                          kind === "ai" ? "ai_responses" : "demo_responses"
                        ];
                      return (
                        <div key={kind}>
                          <span>
                            {i18n.t(
                              "billing." +
                                (kind === "ai"
                                  ? "ai_responses"
                                  : "demo_responses"),
                            )}
                          </span>
                          <strong>
                            {number.format(used)} /{" "}
                            {cap === null
                              ? i18n.t("billing.unlimited")
                              : number.format(cap)}
                          </strong>
                          <progress
                            aria-label={i18n.t(
                              "billing." +
                                (kind === "ai"
                                  ? "ai_responses"
                                  : "demo_responses"),
                            )}
                            value={
                              cap === null
                                ? 0
                                : Math.min(used + reserved, cap || 1)
                            }
                            max={cap === null || cap === 0 ? 1 : cap}
                          />
                          <small>
                            {i18n.t("billing.remaining")}:{" "}
                            {cap === null
                              ? i18n.t("billing.unlimited")
                              : number.format(
                                  Math.max(0, cap - used - reserved),
                                )}
                          </small>
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <p>{c.missing}</p>
                )}
              </div>
              <button onClick={onBilling}>{c.billing} ↗</button>
            </article>
          </>
        )}
      </div>
    </section>
  );
}
