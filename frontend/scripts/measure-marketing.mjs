// Local laboratory measurements, not field Core Web Vitals or GPU capacity.
// Run npm run build, then vite preview --host 127.0.0.1 --port 5174.
import { chromium } from "@playwright/test";
import { mkdir, writeFile } from "node:fs/promises";
const browser = await chromium.launch(),
  results = [];
try {
  for (const width of [1440, 390]) {
    const context = await browser.newContext({
        viewport: { width, height: 1000 },
      }),
      page = await context.newPage();
    await page.addInitScript(() => {
      window.__cosmic = { lcp: 0, cls: 0, longTasks: 0 };
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries())
          window.__cosmic.lcp = entry.startTime;
      }).observe({ type: "largest-contentful-paint", buffered: true });
      new PerformanceObserver((list) => {
        for (const entry of list.getEntries())
          if (!entry.hadRecentInput) window.__cosmic.cls += entry.value;
      }).observe({ type: "layout-shift", buffered: true });
      new PerformanceObserver((list) => {
        window.__cosmic.longTasks += list.getEntries().length;
      }).observe({ type: "longtask", buffered: true });
    });
    const cdp = await context.newCDPSession(page);
    await cdp.send("Network.enable");
    await cdp.send("Network.setCacheDisabled", { cacheDisabled: true });
    await cdp.send("Emulation.setCPUThrottlingRate", { rate: 4 });
    await cdp.send("Network.emulateNetworkConditions", {
      offline: false,
      latency: 40,
      downloadThroughput: 1250000,
      uploadThroughput: 625000,
    });
    await page.goto("http://127.0.0.1:5174/?lang=ru");
    await page.locator("h1").waitFor();
    await page.evaluate(() => document.fonts.ready);
    await page.waitForTimeout(3000);
    results.push(
      await page.evaluate((width) => {
        const resources = performance.getEntriesByType("resource"),
          nav = performance.getEntriesByType("navigation")[0];
        return {
          width,
          ...window.__cosmic,
          fcp: performance.getEntriesByName("first-contentful-paint")[0]
            ?.startTime,
          domReady: nav.domContentLoadedEventEnd,
          transferBytes: resources.reduce(
            (n, r) => n + r.transferSize,
            nav.transferSize,
          ),
          requests: resources.length + 1,
          workspaceLoaded: resources.some((r) => r.name.includes("Workspace")),
          externalRequests: resources.filter(
            (r) => new URL(r.name).origin !== location.origin,
          ).length,
        };
      }, width),
    );
    await context.close();
  }
  const report = {
    date: new Date().toISOString(),
    conditions:
      "Chromium, production Vite preview, cold cache, 4x CPU slowdown, 40ms latency, 10Mbps download; local API; one sample per viewport; no field CWV claim",
    results,
  };
  await mkdir("../artifacts/stage9a2", { recursive: true });
  await writeFile(
    "../artifacts/stage9a2/performance.json",
    JSON.stringify(report, null, 2),
  );
  console.log(JSON.stringify(report, null, 2));
} finally {
  await browser.close();
}
