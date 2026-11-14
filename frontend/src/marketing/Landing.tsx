import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import i18n from "../i18n";
import en from "../locales/en/marketing.json";
import ru from "../locales/ru/marketing.json";
import cs from "../locales/cs/marketing.json";
import de from "../locales/de/marketing.json";
import "./cosmic.css";
for (const [l, dict] of Object.entries({ en, ru, cs, de }))
  i18n.addResourceBundle(l, "marketing", dict, true, true);
type Copy = typeof en.copy;
const sections = ["features", "how", "industries", "plans", "faq"];

export function Mark({ small = false }: { small?: boolean }) {
  return (
    <svg
      className={small ? "cv-mark cv-mark-small" : "cv-mark"}
      viewBox="0 0 44 44"
      aria-hidden="true"
    >
      <path
        d="M9 25V16a10 10 0 0 1 10-10h9a9 9 0 0 1 9 9v10a9 9 0 0 1-9 9H18L9 40v-9"
        fill="currentColor"
        opacity=".24"
      />
      <path
        d="M7 21V14a8 8 0 0 1 8-8h9a8 8 0 0 1 8 8v7a8 8 0 0 1-8 8h-8L7 35V21Z"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.6"
      />
      <path
        d="M16 15h7m-7 6h11"
        stroke="currentColor"
        strokeWidth="2.6"
        strokeLinecap="round"
      />
    </svg>
  );
}
function Arrow({ play = false }: { play?: boolean }) {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      {play ? (
        <path d="m9 5 11 7-11 7V5Z" fill="currentColor" />
      ) : (
        <path
          d="M5 12h14m-6-6 6 6-6 6"
          stroke="currentColor"
          strokeWidth="1.7"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      )}
    </svg>
  );
}
function Check() {
  return (
    <svg width="16" height="16" viewBox="0 0 20 20" aria-hidden="true">
      <path
        d="m4 10 4 4 8-9"
        stroke="currentColor"
        fill="none"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
function Ribbon() {
  const id = useId();
  return (
    <svg className="cv-ribbon" viewBox="0 0 1000 700" aria-hidden="true">
      <defs>
        <linearGradient id={id} x1="0" y1="1" x2="1" y2="0">
          <stop stopColor="#5530C8" />
          <stop offset=".55" stopColor="#AA70F6" />
          <stop offset="1" stopColor="#ED91DD" />
        </linearGradient>
      </defs>
      {Array.from({ length: 19 }, (_, i) => (
        <path
          key={i}
          d={`M ${45 + i * 5} 720 C ${-80 + i * 7} ${280 + i * 4}, ${820 - i * 4} ${630 - i * 6}, 820 ${180 - i * 4} S 1020 10 1050 -80`}
          stroke={`url(#${id})`}
          strokeWidth={i % 6 === 0 ? 2 : 1}
          opacity={0.12 + i * 0.018}
          fill="none"
        />
      ))}
      <ellipse
        cx="600"
        cy="365"
        rx="290"
        ry="155"
        transform="rotate(-28 600 365)"
        fill="none"
        stroke="#D4B9FF"
        strokeOpacity=".12"
      />
    </svg>
  );
}
function LeadCard({ d, compact = false }: { d: Copy; compact?: boolean }) {
  return (
    <div className={`cv-lead-card ${compact ? "cv-compact" : ""}`}>
      <div className="cv-card-caption">
        <span className="cv-status-dot" />
        {d.newLead}
        <span className="cv-lead-spark" aria-hidden="true">
          ↗
        </span>
      </div>
      <div className="cv-person">
        <span className="cv-avatar">{d.person[0]}</span>
        <div>
          <strong>{d.person}</strong>
          <small>{d.interest}</small>
        </div>
      </div>
      {!compact && (
        <div className="cv-contact-row">
          <span>{d.contact}</span>
          <b>hello@example.com</b>
        </div>
      )}
      <div className="cv-confirmed">
        <Check />
        {d.confirmed}
      </div>
    </div>
  );
}
function Chat({ d, hero = false }: { d: Copy; hero?: boolean }) {
  return (
    <div className={`cv-chat ${hero ? "cv-hero-chat" : ""}`}>
      <div className="cv-chat-header">
        <span className="cv-agent-symbol">
          <Mark small />
        </span>
        <div>
          <strong>{d.company}</strong>
          <small>{d.assistant}</small>
        </div>
        <span className="cv-window-dots" aria-hidden="true">
          •••
        </span>
      </div>
      <div className="cv-chat-body">
        <p className="cv-message cv-message-in cv-appear-one">{d.query}</p>
        <div className="cv-reply cv-appear-two">
          <Mark small />
          <div>
            <span className="cv-message-author">{d.assistant}</span>
            <p className="cv-message cv-message-out">{d.answer}</p>
            <span className="cv-source-note">
              <Check />
              {d.source}
            </span>
          </div>
        </div>
      </div>
      <div className="cv-chat-composer" aria-hidden="true">
        <span>{d.placeholder}</span>
        <span className="cv-send">
          <Arrow />
        </span>
      </div>
      <div className="cv-demo-caption">{d.demoLabel}</div>
    </div>
  );
}
function Chart({ d, mini = false }: { d: Copy; mini?: boolean }) {
  const id = useId();
  return (
    <div className={mini ? "cv-mini-chart" : "cv-chart"}>
      {!mini && (
        <div className="cv-chart-top">
          <strong>{d.chartTitle}</strong>
          <span>
            <i />
            {d.chartLegend[0]} <i />
            {d.chartLegend[1]}
          </span>
        </div>
      )}
      <svg
        viewBox="0 0 600 210"
        preserveAspectRatio="none"
        role="img"
        aria-label={d.concept}
      >
        <defs>
          <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
            <stop stopColor="#9957E8" stopOpacity=".25" />
            <stop offset="1" stopColor="#9957E8" stopOpacity="0" />
          </linearGradient>
        </defs>
        {[35, 85, 135, 185].map((y) => (
          <path
            key={y}
            d={`M0 ${y}H600`}
            stroke="#eae5f4"
            strokeDasharray="4 6"
          />
        ))}
        <path
          d="M0 170C40 170 45 130 95 140S160 155 195 105S255 125 300 90S365 100 400 55S470 90 505 40S560 45 600 12V210H0Z"
          fill={`url(#${id})`}
        />
        <path
          className="cv-draw-line"
          d="M0 170C40 170 45 130 95 140S160 155 195 105S255 125 300 90S365 100 400 55S470 90 505 40S560 45 600 12"
          fill="none"
          stroke="#7741D9"
          strokeWidth="4"
          strokeLinecap="round"
        />
        <path
          d="M0 190C80 185 80 175 150 180S245 138 300 145S380 110 425 120S530 85 600 75"
          fill="none"
          stroke="#D690D8"
          strokeWidth="3"
          strokeLinecap="round"
        />
      </svg>
      {!mini && (
        <div className="cv-chart-days">
          {d.days.map((x) => (
            <span key={x}>{x}</span>
          ))}
        </div>
      )}
    </div>
  );
}
function KnowledgeArt({ d }: { d: Copy }) {
  const flowId = useId();
  return (
    <div className="cv-knowledge-art">
      <svg viewBox="0 0 600 270" aria-hidden="true">
        <defs>
          <linearGradient id={flowId}>
            <stop stopColor="#B39BEA" />
            <stop offset="1" stopColor="#5530C8" />
          </linearGradient>
        </defs>
        {[60, 135, 210].map((y, i) => (
          <path
            className="cv-flow-line"
            key={y}
            d={`M200 ${y}C330 ${y} 260 135 390 135`}
            fill="none"
            stroke={`url(#${flowId})`}
            strokeWidth={i === 1 ? 3 : 1.5}
          />
        ))}
      </svg>
      <div className="cv-source-stack">
        {d.documents.map((x, i) => (
          <div className="cv-source-file" key={x}>
            <span className="cv-file-symbol">{["PDF", "DOCX", "WWW"][i]}</span>
            <strong>{x}</strong>
            <Check />
          </div>
        ))}
      </div>
      <div className="cv-knowledge-destination">
        <div className="cv-core">
          <Mark />
        </div>
        <strong>{d.assistant}</strong>
        <small>
          <Check />
          {d.indexed}
        </small>
      </div>
    </div>
  );
}
function Dashboard({ d }: { d: Copy }) {
  const lang = i18n.resolvedLanguage;
  return (
    <div className="cv-dashboard">
      <div className="cv-dashboard-top">
        <span className="cv-panel-brand">
          <Mark small />
          AI Chat
        </span>
        <span className="cv-concept">{d.concept}</span>
      </div>
      <div className="cv-metrics">
        {d.metrics.map((m, i) => (
          <div key={m}>
            <span>{m}</span>
            <strong>
              {i === 2
                ? new Intl.NumberFormat(lang, {
                    style: "percent",
                    maximumFractionDigits: 1,
                  }).format(0.145)
                : new Intl.NumberFormat(lang).format([1280, 186, 0, 42][i])}
            </strong>
            <svg viewBox="0 0 90 23" aria-hidden="true">
              <path
                d="M1 22 16 16 25 18 38 9 48 13 62 7 74 10 89 1"
                fill="none"
                stroke={i === 1 ? "#C87FCB" : "#9C7CDD"}
                strokeWidth="2"
              />
            </svg>
          </div>
        ))}
      </div>
      <Chart d={d} />
    </div>
  );
}
function keyboardTab(
  e: KeyboardEvent,
  index: number,
  count: number,
  select: (n: number) => void,
) {
  let n = index;
  if (e.key === "ArrowRight" || e.key === "ArrowDown") n = (index + 1) % count;
  else if (e.key === "ArrowLeft" || e.key === "ArrowUp")
    n = (index + count - 1) % count;
  else if (e.key === "Home") n = 0;
  else if (e.key === "End") n = count - 1;
  else return;
  e.preventDefault();
  select(n);
  (
    e.currentTarget.parentElement?.querySelectorAll("button")[
      n
    ] as HTMLButtonElement
  )?.focus();
}
function Demo({ d }: { d: Copy }) {
  const [scenario, setScenario] = useState(0),
    [phase, setPhase] = useState(0),
    [replay, setReplay] = useState(0),
    [confirmed, setConfirmed] = useState(false);
  useEffect(() => {
    setPhase(0);
    setConfirmed(false);
    const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
    const timers = [1, 2, 3, 4].map((p) =>
      setTimeout(() => setPhase(p), reduced ? 0 : p * 650),
    );
    return () => timers.forEach(clearTimeout);
  }, [scenario, replay]);
  return (
    <div className="cv-demo-grid">
      <div className="cv-demo-options">
        <div
          role="tablist"
          aria-label={d.demoTitle}
          aria-orientation="vertical"
        >
          {d.scenarios.map((s, i) => (
            <button
              key={s}
              id={`cv-scenario-${i}`}
              role="tab"
              aria-selected={scenario === i}
              aria-controls="cv-demo-panel"
              tabIndex={scenario === i ? 0 : -1}
              onKeyDown={(e) => keyboardTab(e, i, 4, setScenario)}
              onClick={() => setScenario(i)}
            >
              <span>0{i + 1}</span>
              {s}
              <Arrow />
            </button>
          ))}
        </div>
        <p className="cv-honest-note">{d.demoHint}</p>
      </div>
      <div
        className="cv-demo-window"
        id="cv-demo-panel"
        role="tabpanel"
        aria-labelledby={`cv-scenario-${scenario}`}
      >
        <div className="cv-chat-header">
          <span className="cv-agent-symbol">
            <Mark small />
          </span>
          <div>
            <strong>{d.company}</strong>
            <small>
              {scenario === 3 && phase >= 3 ? d.operator : d.assistant}
            </small>
          </div>
          <span className="cv-scenario-badge">{d.demoData}</span>
        </div>
        <div
          className="cv-demo-messages"
          aria-live="polite"
          aria-atomic="false"
        >
          <p className="cv-message cv-message-in">{d.questions[scenario]}</p>
          {phase === 1 && (
            <div className="cv-typing" role="status" aria-label={d.typing}>
              <i />
              <i />
              <i />
            </div>
          )}
          {phase >= 2 && (
            <p className="cv-message cv-message-out cv-pop">
              {d.answers[scenario]}
            </p>
          )}
          {phase >= 3 && scenario === 2 && (
            <div className="cv-confirm-demo cv-pop">
              <LeadCard d={d} />
              <button
                className="cv-button cv-primary"
                onClick={() => setConfirmed(true)}
                disabled={confirmed}
              >
                {confirmed ? <Check /> : null}
                {d.confirmDemo}
              </button>
              {confirmed && <p role="status">{d.demoSuccess}</p>}
            </div>
          )}
          {phase >= 3 && scenario === 3 && (
            <div className="cv-transfer-note cv-pop">
              <Check />
              {d.handoff}
            </div>
          )}
          {phase >= 4 && scenario === 3 && (
            <p className="cv-message cv-message-human cv-pop">
              <strong>
                {d.operatorName} · {d.operator}
              </strong>
              {d.humanAnswer}
            </p>
          )}
        </div>
        <div className="cv-demo-bottom">
          <span>{d.demoLabel}</span>
          <button onClick={() => setReplay((x) => x + 1)}>
            {d.replay}
            <span aria-hidden="true">↻</span>
          </button>
        </div>
      </div>
    </div>
  );
}
function ProductPreview({ d }: { d: Copy }) {
  const [tab, setTab] = useState(0);
  return (
    <div className="cv-product">
      <div className="cv-product-bar">
        <span className="cv-browser-dots" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
        <span>AI Chat / {d.workspace}</span>
        <small>{d.preview}</small>
      </div>
      <div
        className="cv-product-tabs"
        role="tablist"
        aria-label={d.interfaceTitle}
      >
        {d.tabs.map((x, i) => (
          <button
            key={x}
            id={`cv-product-tab-${i}`}
            aria-controls="cv-product-panel"
            role="tab"
            aria-selected={tab === i}
            tabIndex={tab === i ? 0 : -1}
            onKeyDown={(e) => keyboardTab(e, i, 5, setTab)}
            onClick={() => setTab(i)}
          >
            {x}
          </button>
        ))}
      </div>
      <div
        className="cv-product-content"
        role="tabpanel"
        id="cv-product-panel"
        aria-labelledby={`cv-product-tab-${tab}`}
      >
        {tab === 4 ? (
          <Dashboard d={d} />
        ) : tab === 1 ? (
          <div className="cv-knowledge-product">
            <h3>{d.tabs[1]}</h3>
            <KnowledgeArt d={d} />
            <p>{d.features[0].body}</p>
          </div>
        ) : tab === 2 ? (
          <div className="cv-leads-product">
            <LeadCard d={d} />
            <div>
              <h3>{d.tabs[2]}</h3>
              {[d.statusNew, d.statusWork].map((x, i) => (
                <div className="cv-lead-table-row" key={x}>
                  <span className="cv-avatar">{d.person[0]}</span>
                  <strong>{d.person}</strong>
                  <span>{d.interest}</span>
                  <small className="cv-state-pill">{x}</small>
                  <span>{i === 0 ? d.available : d.operatorName}</span>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <div className="cv-inbox-product">
            <aside>
              <strong>{tab === 3 ? d.queue : d.tabs[0]}</strong>
              {[d.visitor, d.person, d.company].map((x, i) => (
                <div key={x} className={i === 0 ? "cv-selected-row" : ""}>
                  <span className="cv-avatar">{x[0]}</span>
                  <div>
                    <b>{x}</b>
                    <small>{i === 0 ? d.questions[3] : d.interest}</small>
                  </div>
                  {i === 0 && <span className="cv-unread">1</span>}
                </div>
              ))}
            </aside>
            <div className="cv-product-conversation">
              <div className="cv-conversation-heading">
                <strong>{d.visitor}</strong>
                <span className="cv-state-pill">
                  {tab === 3 ? d.taken : d.assistant}
                </span>
              </div>
              <p className="cv-message cv-message-in">
                {tab === 3 ? d.questions[3] : d.query}
              </p>
              <p className="cv-message cv-message-out">
                {tab === 3 ? d.humanAnswer : d.answer}
              </p>
              <div className="cv-internal-note">
                <strong>{d.note}</strong>
                <span>{d.humanNote}</span>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export default function Landing() {
  const { t, i18n } = useTranslation("marketing"),
    d = t("copy", { returnObjects: true }) as unknown as Copy;
  const [menu, setMenu] = useState(false),
    [brand, setBrand] = useState("AI Chat"),
    [industry, setIndustry] = useState(0);
  const root = useRef<HTMLDivElement>(null),
    menuButton = useRef<HTMLButtonElement>(null);
  const language = i18n.resolvedLanguage?.split("-")[0] || "en";
  useEffect(() => {
    const lang = new URL(location.href).searchParams.get("lang");
    if (lang && ["ru", "en", "cs", "de"].includes(lang)) {
      localStorage.setItem("locale", lang);
      void i18n.changeLanguage(lang);
    }
    let active = true;
    fetch("/api/v1/config")
      .then((r) => (r.ok ? r.json() : null))
      .then((c) => {
        if (active && c?.brand_name && c.brand_name !== "Site Assistant")
          setBrand(c.brand_name);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [i18n]);
  useEffect(() => {
    document.title = d.title;
    document.documentElement.lang = language;
    const tags: { [key: string]: string } = {
      description: d.description,
      "og:title": d.title,
      "og:description": d.description,
      "og:type": "website",
      "og:locale": language,
      "og:url": location.origin + "/?lang=" + language,
      "og:image": location.origin + "/cosmic-social.png",
      "theme-color": "#140D33",
    };
    for (const [k, v] of Object.entries(tags)) {
      const prop = k.startsWith("og:") ? "property" : "name";
      let el = document.head.querySelector<HTMLMetaElement>(
        `meta[${prop}="${k}"]`,
      );
      if (!el) {
        el = document.createElement("meta");
        el.setAttribute(prop, k);
        document.head.append(el);
      }
      el.content = v;
    }
    let canonical = document.head.querySelector<HTMLLinkElement>(
      "link[rel=canonical]",
    );
    if (!canonical) {
      canonical = document.createElement("link");
      canonical.rel = "canonical";
      document.head.append(canonical);
    }
    canonical.href = location.origin + "/?lang=" + language;
    for (const locale of ["ru", "en", "cs", "de", "x-default"]) {
      let link = document.head.querySelector<HTMLLinkElement>(
        `link[hreflang="${locale}"]`,
      );
      if (!link) {
        link = document.createElement("link");
        link.rel = "alternate";
        link.hreflang = locale;
        document.head.append(link);
      }
      link.href =
        location.origin + "/?lang=" + (locale === "x-default" ? "en" : locale);
    }
  }, [d, language]);
  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        for (const e of entries)
          if (e.isIntersecting) {
            e.target.classList.add("cv-visible");
            observer.unobserve(e.target);
          }
      },
      { threshold: 0.06 },
    );
    root.current
      ?.querySelectorAll(".cv-reveal")
      .forEach((el) => observer.observe(el));
    return () => observer.disconnect();
  }, []);
  function changeLanguage(value: string) {
    localStorage.setItem("locale", value);
    void i18n.changeLanguage(value);
    history.replaceState(null, "", "?lang=" + value + location.hash);
  }
  const signup = (label = d.start, light = false) => (
    <a
      className={`cv-button ${light ? "cv-light-button" : "cv-primary"}`}
      href="/register"
    >
      {label}
      <Arrow />
    </a>
  );
  return (
    <div
      className="cosmic"
      ref={root}
      onKeyDown={(e) => {
        if (e.key === "Escape" && menu) {
          setMenu(false);
          menuButton.current?.focus();
        }
      }}
    >
      <a href="#main" className="cv-skip">
        {d.skip}
      </a>
      <header className="cv-header">
        <div className="cv-wrap cv-header-inner">
          <a href="#main" className="cv-brand" aria-label={brand}>
            <Mark />
            {brand}
            <span className="cv-brand-point" />
          </a>
          <nav
            aria-label={d.menu}
            className={menu ? "cv-nav cv-nav-open" : "cv-nav"}
          >
            {d.nav.map((x, i) => (
              <a
                key={x}
                href={"#" + sections[i]}
                onClick={() => setMenu(false)}
              >
                {x}
              </a>
            ))}
            <a href="/login" className="cv-mobile-login">
              {d.login}
            </a>
          </nav>
          <div className="cv-header-right">
            <select
              aria-label={d.language}
              value={language}
              onChange={(e) => changeLanguage(e.target.value)}
            >
              {["ru", "en", "cs", "de"].map((l) => (
                <option key={l} value={l}>
                  {l.toUpperCase()}
                </option>
              ))}
            </select>
            <a className="cv-login" href="/login">
              {d.login}
            </a>
            <a className="cv-nav-cta" href="/register">
              {d.start}
              <Arrow />
            </a>
            <button
              ref={menuButton}
              className="cv-menu-toggle"
              aria-label={menu ? d.close : d.menu}
              aria-expanded={menu}
              onClick={() => setMenu(!menu)}
            >
              <span />
              <span />
            </button>
          </div>
        </div>
      </header>
      <main id="main">
        <section className="cv-hero">
          <div className="cv-hero-light" />
          <Ribbon />
          <div className="cv-wrap cv-hero-grid">
            <div className="cv-hero-copy">
              <div className="cv-eyebrow cv-hero-eyebrow">
                <span />
                {d.eyebrow}
              </div>
              <h1 className="cv-title">
                {d.hero.map((x, i) => (
                  <span key={x} className={i === 2 ? "cv-gradient-text" : ""}>
                    {x}
                  </span>
                ))}
              </h1>
              <p className="cv-hero-body">{d.heroBody}</p>
              <div className="cv-actions">
                {signup(d.try, true)}
                <a href="#demo" className="cv-button cv-ghost">
                  <Arrow play />
                  {d.watch}
                </a>
              </div>
              <p className="cv-hero-note">
                <span className="cv-check-ring">
                  <Check />
                </span>
                {d.heroNote}
              </p>
            </div>
            <div className="cv-hero-stage">
              <div className="cv-stage-orbit" />
              <div className="cv-stage-tag">
                <span />
                {d.pilot}
              </div>
              <Chat d={d} hero />
              <div className="cv-floating-lead">
                <LeadCard d={d} compact />
              </div>
              <div className="cv-floating-chart">
                <span>{d.demoData}</span>
                <strong>+24</strong>
                <Chart d={d} mini />
              </div>
              <div className="cv-hand-card">
                <span className="cv-human-avatar">{d.operatorName[0]}</span>
                <span>{d.handoff}</span>
                <Check />
              </div>
            </div>
          </div>
          <div className="cv-wrap cv-flow-strip">
            {d.flow.map((x, i) => (
              <div key={x}>
                <span>0{i + 1}</span>
                <strong>{x}</strong>
                {i < 3 && <Arrow />}
              </div>
            ))}
          </div>
        </section>
        <section className="cv-section cv-pain cv-reveal">
          <div className="cv-wrap cv-pain-grid">
            <div>
              <span className="cv-section-no">01 / {d.nav[0]}</span>
              <h2>{d.painTitle}</h2>
              <p className="cv-section-body">{d.painBody}</p>
              <div className="cv-conversation-trail" aria-hidden="true">
                <span />
                <i />
                <i />
                <i />
                <span />
              </div>
            </div>
            <div className="cv-pain-list">
              {d.pains.map((x, i) => (
                <article key={x.title}>
                  <span>0{i + 1}</span>
                  <div>
                    <h3>{x.title}</h3>
                    <p>{x.body}</p>
                  </div>
                  <span className="cv-pain-line" aria-hidden="true" />
                </article>
              ))}
            </div>
          </div>
        </section>
        <section id="features" className="cv-section cv-features">
          <div className="cv-wrap">
            <div className="cv-heading cv-reveal">
              <span className="cv-eyebrow">{d.featureEyebrow}</span>
              <h2>{d.featureTitle}</h2>
            </div>
            <div className="cv-bento">
              <article className="cv-feature cv-knowledge-feature cv-reveal">
                <span className="cv-feature-index">01</span>
                <h3>{d.features[0].title}</h3>
                <p>{d.features[0].body}</p>
                <KnowledgeArt d={d} />
              </article>
              <article className="cv-feature cv-answer-feature cv-reveal">
                <span className="cv-feature-index">02</span>
                <h3>{d.features[1].title}</h3>
                <p>{d.features[1].body}</p>
                <div className="cv-answer-visual">
                  <div>
                    <Mark small />
                    <span>{d.assistant}</span>
                  </div>
                  <p>{d.greeting}</p>
                  <span>
                    <Check />
                    {d.source}
                  </span>
                </div>
              </article>
              <article className="cv-feature cv-lead-feature cv-reveal">
                <span className="cv-feature-index">03</span>
                <h3>{d.features[2].title}</h3>
                <p>{d.features[2].body}</p>
                <LeadCard d={d} compact />
              </article>
              <article className="cv-feature cv-history-feature cv-reveal">
                <span className="cv-feature-index">04</span>
                <h3>{d.features[3].title}</h3>
                <p>{d.features[3].body}</p>
                <div className="cv-history-art">
                  <span className="cv-history-line" />
                  <div>
                    <i />
                    <span>
                      {d.today}
                      <b>{d.visitor}</b>
                    </span>
                  </div>
                  <div>
                    <i />
                    <span>
                      {d.history}
                      <b>{d.assistant}</b>
                    </span>
                  </div>
                  <div>
                    <i />
                    <span>
                      {d.note}
                      <b>{d.operatorName}</b>
                    </span>
                  </div>
                </div>
              </article>
              <article className="cv-feature cv-team-feature cv-reveal">
                <span className="cv-feature-index">05</span>
                <h3>{d.features[4].title}</h3>
                <p>{d.features[4].body}</p>
                <div className="cv-people-art">
                  <span className="cv-agent-symbol">
                    <Mark />
                  </span>
                  <span className="cv-bridge" />
                  <span className="cv-human-avatar">{d.operatorName[0]}</span>
                </div>
                <span className="cv-team-ack">
                  <Check />
                  {d.taken}
                </span>
              </article>
            </div>
          </div>
        </section>
        <section id="how" className="cv-section cv-how cv-reveal">
          <div className="cv-wrap">
            <div className="cv-heading">
              <span className="cv-section-no">02 / {d.nav[1]}</span>
              <h2>{d.howTitle}</h2>
            </div>
            <div className="cv-steps">
              {d.steps.map((s, i) => (
                <article key={s.title}>
                  <div className="cv-step-number">
                    0{i + 1}
                    <span className="cv-step-connector" />
                  </div>
                  <h3>{s.title}</h3>
                  <p>{s.body}</p>
                </article>
              ))}
            </div>
          </div>
        </section>
        <section id="demo" className="cv-section cv-demo-section cv-reveal">
          <div className="cv-wrap">
            <div className="cv-heading">
              <span className="cv-eyebrow">{d.demoLabel}</span>
              <h2>{d.demoTitle}</h2>
              <p className="cv-section-body">{d.demoBody}</p>
            </div>
            <Demo d={d} />
          </div>
        </section>
        <section className="cv-section cv-interfaces cv-reveal">
          <div className="cv-wrap">
            <div className="cv-heading">
              <span className="cv-section-no">03 / AI Chat</span>
              <h2>{d.interfaceTitle}</h2>
              <p className="cv-section-body">{d.interfaceBody}</p>
            </div>
            <ProductPreview d={d} />
          </div>
        </section>
        <section className="cv-human-section cv-reveal">
          <div className="cv-wrap cv-human-grid">
            <div>
              <span className="cv-eyebrow">{d.operator}</span>
              <h2>{d.humanTitle}</h2>
              <p>{d.humanBody}</p>
              <span className="cv-human-note">{d.humanNote}</span>
            </div>
            <div className="cv-handoff-art">
              {d.humanSteps.map((s, i) => (
                <div key={s} className={"cv-handoff-step cv-handoff-step-" + i}>
                  <span
                    className={i < 2 ? "cv-agent-symbol" : "cv-human-avatar"}
                  >
                    {i < 2 ? <Mark small /> : d.operatorName[0]}
                  </span>
                  <div>
                    <small>0{i + 1}</small>
                    <strong>{s}</strong>
                  </div>
                  {i === 2 && <Check />}
                </div>
              ))}
            </div>
          </div>
        </section>
        <section id="industries" className="cv-section cv-industry cv-reveal">
          <div className="cv-wrap">
            <div className="cv-heading">
              <span className="cv-section-no">04 / {d.nav[2]}</span>
              <h2>{d.businessTitle}</h2>
              <p className="cv-section-body">{d.businessBody}</p>
            </div>
            <div className="cv-industry-grid">
              <div
                className="cv-industry-options"
                role="tablist"
                aria-label={d.businessTitle}
              >
                {d.industries.map((x, i) => (
                  <button
                    key={x.name}
                    id={`cv-industry-${i}`}
                    role="tab"
                    aria-selected={industry === i}
                    aria-controls="cv-industry-example"
                    tabIndex={industry === i ? 0 : -1}
                    onKeyDown={(e) => keyboardTab(e, i, 6, setIndustry)}
                    onClick={() => setIndustry(i)}
                  >
                    <span>0{i + 1}</span>
                    {x.name}
                    <Arrow />
                  </button>
                ))}
              </div>
              <div
                className="cv-industry-example"
                id="cv-industry-example"
                role="tabpanel"
                aria-labelledby={`cv-industry-${industry}`}
              >
                <div className="cv-industry-thread" aria-hidden="true">
                  <Mark />
                </div>
                <span className="cv-eyebrow">{d.demoData}</span>
                <h3>{d.industries[industry].name}</h3>
                <p>{d.industries[industry].example}</p>
                <div className="cv-industry-tags">
                  {[d.flow[0], d.flow[1], d.flow[2]].map((x) => (
                    <span key={x}>
                      <Check />
                      {x}
                    </span>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </section>
        <section className="cv-section cv-integration cv-reveal">
          <div className="cv-wrap cv-integration-grid">
            <div>
              <span className="cv-eyebrow">
                {d.api} / {d.webhook}
              </span>
              <h2>{d.integrationTitle}</h2>
              <p className="cv-section-body">{d.integrationBody}</p>
              <small>{d.integrationNote}</small>
            </div>
            <div className="cv-api-art">
              <div className="cv-api-node cv-api-origin">
                <Mark />
                <strong>AI Chat</strong>
              </div>
              <div className="cv-api-link">
                <svg viewBox="0 0 200 130" aria-hidden="true">
                  <path
                    d="M0 65C80 65 90 15 200 15M0 65C80 65 90 115 200 115"
                    stroke="#AA84E4"
                    fill="none"
                    strokeWidth="2"
                    strokeDasharray="4 5"
                  />
                </svg>
                <span>{d.webhook}</span>
                <span>{d.api}</span>
              </div>
              <div className="cv-api-node cv-api-target">
                <svg
                  width="35"
                  height="35"
                  viewBox="0 0 40 40"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.8"
                  aria-hidden="true"
                >
                  <rect x="4" y="7" width="32" height="26" rx="5" />
                  <path d="M4 15h32M14 15v18M21 22h9m-9 5h6" />
                </svg>
                <strong>{d.crm}</strong>
              </div>
            </div>
          </div>
        </section>
        <section className="cv-analytics-section cv-reveal">
          <div className="cv-wrap">
            <div className="cv-analytics-heading">
              <div>
                <span className="cv-eyebrow">{d.concept}</span>
                <h2>{d.analyticsTitle}</h2>
              </div>
              <p>{d.analyticsBody}</p>
            </div>
            <Dashboard d={d} />
          </div>
        </section>
        <section id="plans" className="cv-section cv-pricing cv-reveal">
          <div className="cv-wrap">
            <div className="cv-heading">
              <span className="cv-section-no">05 / {d.nav[3]}</span>
              <h2>{d.priceTitle}</h2>
              <p className="cv-section-body">{d.priceBody}</p>
            </div>
            <div className="cv-plans">
              {d.plans.map((p, i) => (
                <article
                  key={p.name}
                  className={i === 1 ? "cv-plan cv-plan-featured" : "cv-plan"}
                >
                  <span className="cv-plan-tag">{p.tag}</span>
                  <h3>{p.name}</h3>
                  <p>{p.body}</p>
                  <div className="cv-price-pending">{d.pricePending}</div>
                  {signup()}
                  <ul>
                    {d.planFeatures.map((x) => (
                      <li key={x}>
                        <Check />
                        {x}
                      </li>
                    ))}
                  </ul>
                </article>
              ))}
            </div>
            <p className="cv-plan-note">{d.planNote}</p>
          </div>
        </section>
        <section id="faq" className="cv-section cv-faq cv-reveal">
          <div className="cv-wrap cv-faq-grid">
            <div>
              <span className="cv-section-no">06 / FAQ</span>
              <h2>{d.faqTitle}</h2>
              <div className="cv-faq-mark" aria-hidden="true">
                <Mark />
              </div>
            </div>
            <div>
              {d.faqs.map((f, i) => (
                <details key={f.q}>
                  <summary>
                    <span>0{i + 1}</span>
                    {f.q}
                    <i aria-hidden="true" />
                  </summary>
                  <p>{f.a}</p>
                </details>
              ))}
            </div>
          </div>
        </section>
        <section className="cv-final cv-reveal">
          <Ribbon />
          <div className="cv-wrap">
            <span className="cv-final-mark">
              <Mark />
            </span>
            <h2>{d.finalTitle}</h2>
            <p>{d.finalBody}</p>
            {signup(d.start, true)}
          </div>
        </section>
      </main>
      <footer className="cv-footer">
        <div className="cv-wrap">
          <div className="cv-footer-top">
            <a className="cv-brand" href="#main">
              <Mark />
              {brand}
            </a>
            <p>{d.footer}</p>
            <a href="#main">
              {d.backTop}
              <span aria-hidden="true">↑</span>
            </a>
          </div>
          <div className="cv-footer-bottom">
            <span>
              © {new Date().getFullYear()} {brand} · {d.rights}
            </span>
            <span>{d.footerNote}</span>
          </div>
        </div>
      </footer>
    </div>
  );
}
