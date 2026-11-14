import { test, expect } from "@playwright/test";
test("new organization receives real Trial usage, independent company quotas and localized responsive billing", async ({
  page,
}) => {
  test.setTimeout(120000);
  const headers = { Origin: "http://localhost:5173", "X-CSRF-Protection": "1" };
  async function post(path: string, data: unknown) {
    const r = await page.request.post("/api/v1" + path, { headers, data });
    expect(r.ok(), await r.text()).toBe(true);
    return r.json();
  }
  const email = `billing-${Date.now()}@example.com`,
    password = "billing-browser-password";
  await post("/auth/register", { email, password, preferred_locale: "en" });
  await post("/auth/login", { email, password });
  const org = await post("/organizations", {
      name: "Billing Studio",
      country: "CZ",
    }),
    other = await post("/organizations", {
      name: "Independent company",
      country: "CZ",
    });
  const path = `/organizations/${org.id}`;
  const assistant = await post(path + "/assistants", { name: "Guide" });
  await post(path + "/sites", {
    name: "Studio",
    domain: "billing.example",
    assistant_id: assistant.id,
  });
  const conv = await post(
    path + `/assistants/${assistant.id}/preview/conversations`,
    {},
  );
  const reply = await page.request.post(
    "/api/v1" +
      path +
      `/assistants/${assistant.id}/preview/conversations/${conv.id}/messages`,
    { headers, data: { request_id: crypto.randomUUID(), content: "Hello" } },
  );
  expect(reply.ok(), await reply.text()).toBe(true);
  const data = await (
    await page.request.get("/api/v1" + path + "/billing")
  ).json();
  expect(data.subscription.status).toBe("trialing");
  expect(data.plan.code).toBe("trial");
  expect(data.period.ai_used).toBe(0);
  expect(data.period.demo_used).toBe(1);
  expect(data.resources.sites).toBe(1);
  expect(data.resources.members).toBe(1);
  expect(
    new Date(data.subscription.trial_end).getTime() -
      new Date(data.subscription.started_at).getTime(),
  ).toBe(14 * 86400000);
  const independent = await (
    await page.request.get(`/api/v1/organizations/${other.id}/billing`)
  ).json();
  expect(independent.period.demo_used).toBe(0);
  expect(independent.resources.assistants).toBe(0);
  for (let i = 0; i < 2; i++)
    await post(path + "/assistants", { name: "Additional " + i });
  const denied = await page.request.post("/api/v1" + path + "/assistants", {
    headers,
    data: { name: "Over quota" },
  });
  expect(denied.status()).toBe(409);
  expect((await denied.json()).error.code).toBe("plan_limit_exceeded");
  await page.goto("/app");
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", { name: "Billing Studio", exact: true }),
    })
    .click();
  await page
    .getByRole("button", { name: "Plan and usage", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Trial", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".billing-meters")).toContainText("3 / 3");
  await expect(page.locator(".billing-meters")).toContainText("0 / 100");
  await expect(page.locator(".billing-meters")).toContainText("1 / 500");
  expect(await page.getByRole("button", { name: /pay|buy/i }).count()).toBe(0);
  for (const [locale, title] of [
    ["ru", "Тариф и использование"],
    ["cs", "Tarif a využití"],
    ["de", "Tarif und Nutzung"],
    ["en", "Plan and usage"],
  ]) {
    await page.locator("header select").first().selectOption(locale);
    await expect(
      page.getByRole("heading", { name: title, exact: true }),
    ).toBeVisible();
  }
  for (const width of [360, 768, 1440]) {
    await page.setViewportSize({ width, height: 960 });
    expect(
      await page
        .locator("html")
        .evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    ).toBe(true);
    await page.screenshot({
      path: `../artifacts/stage8/billing-${width}.png`,
      fullPage: true,
    });
  }
  await page.reload();
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", { name: "Billing Studio", exact: true }),
    })
    .click();
  await page
    .getByRole("button", { name: "Plan and usage", exact: true })
    .click();
  await expect(page.locator(".billing-meters")).toContainText("1 / 500");
});
