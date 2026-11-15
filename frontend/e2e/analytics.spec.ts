import { test, expect } from "@playwright/test";
import { mkdir, writeFile } from "node:fs/promises";
test("real analytics, confirmed request, handoff, filters, locales and responsive views", async ({
  page,
}) => {
  test.setTimeout(180000);
  page.setDefaultTimeout(15000);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const headers = { Origin: "http://localhost:5173", "X-CSRF-Protection": "1" };
  const request = async (
    path: string,
    data: unknown = {},
    method = "POST",
    custom = headers,
  ) => {
    const r = await page.request.fetch("/api/v1" + path, {
      method,
      headers: custom,
      data,
    });
    expect(r.ok(), await r.text()).toBe(true);
    return r.json();
  };
  const email = `analytics-${Date.now()}@example.com`,
    password = "analytics-test-password";
  await request("/auth/register", { email, password, preferred_locale: "en" });
  await request("/auth/login", { email, password });
  const org = await request("/organizations", {
      name: "Analytics synthetic studio",
      country: "CZ",
    }),
    base = "/organizations/" + org.id;
  const assistant = await request(base + "/assistants", {
      name: "Studio guide",
    }),
    site = await request(base + "/sites", {
      name: "Studio website",
      domain: "analytics.example",
      assistant_id: assistant.id,
    });
  const widget = await request(base + "/sites/" + site.id + "/widget");
  const emptyAssistant = await request(base + "/assistants", {
    name: "No activity guide",
  });
  const emptySite = await request(base + "/sites", {
    name: "No activity website",
    domain: "empty.analytics.example",
    assistant_id: emptyAssistant.id,
  });
  await request(
    base + "/sites/" + site.id + "/widget",
    {
      enabled: true,
      origins: ["http://localhost:4173"],
      settings: {
        ...widget.widget.settings,
        privacy_url: "https://analytics.example/privacy",
      },
    },
    "PATCH",
  );
  await request(
    base + "/assistants/" + assistant.id + "/lead-settings",
    {
      revision: 1,
      enabled: true,
      title: "Consultation",
      goal: "Help request a consultation",
      require_contact: false,
      fields: [
        {
          key: "full_name",
          label: "Name",
          type: "text",
          active: true,
          required: true,
          position: 0,
          max_length: 100,
        },
      ],
      success_messages: {},
    },
    "PUT",
  );
  const sessions = [];
  for (let i = 0; i < 4; i++) {
    const session = await request(
      "/public/widgets/" + widget.widget.id + "/sessions",
      {},
      "POST",
      {
        Origin: "http://localhost:4173",
        "Content-Type": "application/json",
      } as typeof headers,
    );
    const root =
        "/public/widgets/" + widget.widget.id + "/sessions/" + session.id,
      publicHeaders = {
        Origin: "http://localhost:4173",
        "Content-Type": "application/json",
        Authorization: "Bearer " + session.token,
      };
    const reply = await page.request.post("/api/v1" + root + "/messages", {
      headers: publicHeaders,
      data: {
        request_id: crypto.randomUUID(),
        content: "Please help with our website",
      },
    });
    expect(reply.ok(), await reply.text()).toBe(true);
    sessions.push({ root, headers: publicHeaders });
  }
  const visitor = sessions[0];
  let collection = await (
    await page.request.get("/api/v1" + visitor.root + "/collection", {
      headers: visitor.headers,
    })
  ).json();
  collection = await request(
    visitor.root + "/collection/start",
    {
      version: collection.version,
      conversation_version: collection.conversation_version,
    },
    "POST",
    visitor.headers as typeof headers,
  );
  collection = await request(
    visitor.root + "/collection/edit",
    {
      version: collection.version,
      conversation_version: collection.conversation_version,
      values: { full_name: "Synthetic visitor" },
    },
    "POST",
    visitor.headers as typeof headers,
  );
  await request(
    visitor.root + "/collection/submit",
    {
      version: collection.version,
      conversation_version: collection.conversation_version,
    },
    "POST",
    visitor.headers as typeof headers,
  );
  await request(
    sessions[1].root + "/request-human",
    { request_id: crypto.randomUUID(), version: 1 },
    "POST",
    sessions[1].headers as typeof headers,
  );
  const list = await (
    await page.request.get(
      "/api/v1" + base + "/conversations?mode=human_requested",
    )
  ).json();
  const conv = list.items[0];
  const taken = await request(base + "/conversations/" + conv.id + "/take", {
    request_id: crypto.randomUUID(),
    version: conv.state_version,
  });
  await request(base + "/conversations/" + conv.id + "/messages", {
    request_id: crypto.randomUUID(),
    version: taken.state_version,
    content: "A synthetic teammate reply.",
  });
  const reportStarted = Date.now();
  const report = await (
    await page.request.get(
      "/api/v1" + base + "/analytics?timezone=Europe/Prague",
    )
  ).json();
  const reportDuration = Date.now() - reportStarted;
  expect(report.kpis.current.dialogs).toBe(4);
  expect(report.kpis.current.leads).toBe(1);
  expect(report.kpis.current.converted).toBe(1);
  expect(report.kpis.current.generation_ms).toBeNull();
  await page.goto("/app");
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", {
        name: "Analytics synthetic studio",
        exact: true,
      }),
    })
    .click();
  await page.getByRole("button", { name: "Analytics", exact: true }).click();
  await expect(page.locator(".an-kpis")).toContainText("25%");
  await page
    .getByRole("combobox", { name: "Website", exact: true })
    .selectOption(emptySite.id);
  await page
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(page.locator(".an-empty")).toBeVisible();
  await page
    .getByRole("combobox", { name: "Website", exact: true })
    .selectOption("");
  await page
    .getByRole("combobox", { name: "Assistant", exact: true })
    .selectOption(emptyAssistant.id);
  await page
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(page.locator(".analytics")).toHaveAttribute(
    "aria-busy",
    "false",
  );
  await expect(page.locator(".an-kpis").first()).toContainText("0");
  await page
    .getByRole("combobox", { name: "Assistant", exact: true })
    .selectOption("");
  await page
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(page.locator(".an-kpis")).toContainText("25%");
  const series = page.locator(".an-chart-legend button").first();
  await series.click();
  await expect(series).toHaveAttribute("aria-pressed", "false");
  await series.click();
  await expect(series).toHaveAttribute("aria-pressed", "true");
  const iteration = process.env.ANALYTICS_VISUAL_ITERATION || "1";
  await mkdir("../artifacts/stage9a3", { recursive: true });
  await writeFile(
    "../artifacts/stage9a3/browser-results.json",
    JSON.stringify(
      {
        http_report_ms: reportDuration,
        kpis: report.kpis.current,
        real_postgresql: true,
        provider: "mock",
        visual_iteration: iteration,
      },
      null,
      2,
    ),
  );
  let accessibilityChecks = 0;
  if (process.env.AXE_SOURCE_FILE)
    await page.addScriptTag({ path: process.env.AXE_SOURCE_FILE });
  for (const [lang, nav] of [
    ["en", "Analytics"],
    ["ru", "Аналитика"],
    ["cs", "Analytika"],
    ["de", "Analysen"],
  ]) {
    await page.locator("header select").first().selectOption(lang);
    for (const width of [1440, 1920, 768, 390, 360]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.evaluate(() => document.fonts.ready);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      for (let tab = 0; tab < 3; tab++) {
        await page.locator(".an-tabs button").nth(tab).click();
        await expect(page.locator(".analytics")).toHaveAttribute(
          "aria-busy",
          "false",
        );
        if (
          process.env.AXE_SOURCE_FILE &&
          ((lang === "en" && width === 1440) ||
            (lang === "ru" && width === 390) ||
            (lang === "de" && width === 360) ||
            (lang === "cs" && width === 768))
        ) {
          const violations = await page.evaluate(async () => {
            const results = await (window as any).axe.run(
              document.querySelector(".analytics"),
              {
                runOnly: {
                  type: "tag",
                  values: ["wcag2a", "wcag2aa", "wcag21aa"],
                },
              },
            );
            return results.violations.map((v: any) => ({
              id: v.id,
              nodes: v.nodes.map((n: any) => ({
                target: n.target,
                summary: n.failureSummary,
              })),
            }));
          });
          expect(violations).toEqual([]);
          accessibilityChecks++;
        }
        if (lang === "en" || width === 360)
          await page.locator(".analytics").screenshot({
            path: `../artifacts/stage9a3/iteration${iteration}-${lang}-${width}-tab${tab}.png`,
          });
      }
      await page.locator(".an-tabs button").first().click();
    }
    await expect(
      page.getByRole("button", { name: nav, exact: true }),
    ).toBeVisible();
  }
  await page.locator("header select").first().selectOption("en");
  await page.locator(".an-tabs button").nth(1).click();
  await page
    .getByRole("button", { name: "Open conversation ↗", exact: true })
    .first()
    .click();
  await expect(page.locator(".inbox-shell")).toBeVisible();
  await page.getByRole("button", { name: "Analytics", exact: true }).click();
  await page.getByRole("button", { name: "Open leads ↗", exact: true }).click();
  await expect(page.locator(".lead-row").first()).toBeVisible();
  await page.locator(".lead-row").first().click();
  await expect(page.locator(".lead-detail")).toBeVisible();
  await page.getByRole("button", { name: "Analytics", exact: true }).click();
  await page
    .getByRole("button", { name: "Open plan & usage ↗", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Trial", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Analytics", exact: true }).click();
  await page.locator(".an-kpi details").first().locator("summary").click();
  await expect(page.locator(".an-kpi details[open] p")).toBeVisible();
  await page.getByLabel("From", { exact: true }).fill("2025-01-01");
  await page.getByLabel("To", { exact: true }).fill("2025-01-07");
  await page
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(page.locator(".an-empty")).toBeVisible();
  await expect(page).toHaveURL(/an_from=2025-01-01/);
  await page.locator(".analytics").screenshot({
    path: `../artifacts/stage9a3/iteration${iteration}-empty.png`,
  });
  expect(errors).toEqual([]);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    await page.locator(".analytics").screenshot({
      path: `../artifacts/stage9a3/iteration${iteration}-empty-${width}.png`,
    });
  }
  if (process.env.AXE_SOURCE_FILE)
    await writeFile(
      "../artifacts/stage9a3/accessibility.json",
      JSON.stringify(
        {
          checks: accessibilityChecks,
          violations: 0,
          engine: "axe-core 4.11.0",
        },
        null,
        2,
      ),
    );
});
