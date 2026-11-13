import { uuid, type Config, type Session } from "./types";
import { validBridge, safeSession } from "./bridge";
(() => {
  const script = document.currentScript as HTMLScriptElement | null;
  if (!script) return;
  const id = script.dataset.widgetId || "";
  if (!uuid.test(id) || document.querySelector(`[data-sa-widget="${id}"]`))
    return;
  const server = new URL(script.src).origin;
  if (!["http:", "https:"].includes(location.protocol)) return;
  const parentOrigin = location.origin;
  const channel = crypto.randomUUID();
  let config: Config;
  let frame: HTMLIFrameElement | null = null;
  let open = false;
  let session: Session | null = null;
  const key = "site-assistant:" + id;
  const host = document.createElement("div");
  host.dataset.saWidget = id;
  host.style.setProperty("display", "none", "important");
  document.documentElement.append(host);
  const root = host.attachShadow({ mode: "open" });
  const button = document.createElement("button");
  button.className = "sa-launcher";
  button.type = "button";
  button.innerHTML =
    '<svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M20 11.5a8 8 0 0 1-8 8H5l-3 2V11.5a9 9 0 1 1 18 0Z" stroke="currentColor" stroke-width="1.7" stroke-linejoin="round"/><path d="M7 10h8M7 14h5" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>';
  const send = (type: string, data?: unknown) =>
    frame?.contentWindow?.postMessage({ type, channel, data }, server);
  function layout() {
    if (!config) return;
    host.style.setProperty("position", "fixed", "important");
    host.style.setProperty(
      config.settings.position,
      config.settings.offset_x + "px",
      "important",
    );
    host.style.setProperty(
      "bottom",
      config.settings.offset_y + "px",
      "important",
    );
    host.style.setProperty("z-index", "2147483000", "important");
    root.host instanceof HTMLElement &&
      root.host.style.setProperty("--launch", config.settings.launcher_color);
    button.style.color = contrast(config.settings.launcher_color);
    if (frame) {
      frame.style[config.settings.position] = config.settings.offset_x + "px";
      frame.style.bottom = config.settings.offset_y + 76 + "px";
      if (innerWidth <= 600) {
        frame.style.top = (visualViewport?.offsetTop || 0) + "px";
        frame.style.height = (visualViewport?.height || innerHeight) + "px";
        frame.style.bottom = "auto";
      } else {
        frame.style.top = "auto";
        frame.style.height = "640px";
        frame.style.bottom = config.settings.offset_y + 76 + "px";
      }
    }
  }
  function toggle(value: boolean) {
    open = value;
    if (!frame && value) {
      frame = document.createElement("iframe");
      frame.className = "sa-frame";
      frame.title = label(config.settings.default_locale);
      frame.setAttribute("sandbox", "allow-scripts allow-same-origin");
      frame.setAttribute("referrerpolicy", "no-referrer");
      frame.src =
        server +
        "/widget/" +
        id +
        "?parent_origin=" +
        encodeURIComponent(parentOrigin) +
        "&channel=" +
        channel;
      root.append(frame);
      layout();
    }
    if (frame) frame.hidden = !value;
    button.hidden = value && innerWidth <= 600;
    button.setAttribute("aria-expanded", String(value));
    send("visibility", { open: value });
    if (!value) button.focus();
  }
  window.addEventListener("message", (event) => {
    if (!frame || !validBridge(event, frame.contentWindow, server, channel))
      return;
    switch (event.data.type) {
      case "ready":
        send("init", {
          session: config.settings.remember_session ? session : null,
        });
        send("visibility", { open });
        break;
      case "close":
        toggle(false);
        break;
      case "session":
        if (event.data.data === null) {
          session = null;
          try {
            sessionStorage.removeItem(key);
          } catch {}
        } else if (safeSession(event.data.data)) {
          session = event.data.data;
          if (config.settings.remember_session)
            try {
              sessionStorage.setItem(key, JSON.stringify(session));
            } catch {}
        }
        break;
    }
  });
  window.addEventListener("resize", layout);
  visualViewport?.addEventListener("resize", layout);
  visualViewport?.addEventListener("scroll", layout);
  window.addEventListener("pagehide", () =>
    send("visibility", { open: false }),
  );
  button.addEventListener("click", () => toggle(!open));
  void fetch(server + "/api/v1/public/widgets/" + id + "/config", {
    credentials: "omit",
    referrerPolicy: "no-referrer",
  })
    .then(async (r) => {
      if (!r.ok) throw Error();
      config = await r.json();
      if (!config.settings.show_launcher) return;
      try {
        const saved = JSON.parse(sessionStorage.getItem(key) || "null");
        if (config.settings.remember_session && safeSession(saved))
          session = saved;
      } catch {}
      const link = document.createElement("link");
      link.rel = "stylesheet";
      link.href = server + "/widget/loader.css";
      root.append(link, button);
      button.setAttribute(
        "aria-label",
        label(
          config.settings.auto_locale
            ? navigator.language.split("-")[0]
            : config.settings.default_locale,
        ),
      );
      button.setAttribute("aria-expanded", "false");
      button.title = button.getAttribute("aria-label") || "";
      layout();
      host.style.setProperty("display", "block", "important");
    })
    .catch(() => {});
  function label(l: string) {
    return (
      (
        {
          ru: "Открыть чат",
          en: "Open chat",
          cs: "Otevřít chat",
          de: "Chat öffnen",
        } as Record<string, string>
      )[l] || "Open chat"
    );
  }
  function contrast(hex: string) {
    const v = hex.replace("#", "");
    const rgb = [0, 2, 4]
      .map((i) => parseInt(v.slice(i, i + 2), 16) / 255)
      .map((x) => (x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4));
    return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722 > 0.179
      ? "#14233b"
      : "#ffffff";
  }
})();
