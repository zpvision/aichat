import { test, expect } from "@playwright/test";
import http from "node:http";
import { createHmac, timingSafeEqual } from "node:crypto";
test("universal CRM API, signed local webhook retry and one-time secrets", async ({
  page,
}) => {
  test.setTimeout(120000);
  const headers = { Origin: "http://localhost:5173", "X-CSRF-Protection": "1" };
  async function post(path: string, data: unknown) {
    const r = await page.request.post("/api/v1" + path, { headers, data });
    expect(r.ok(), await r.text()).toBe(true);
    return r.json();
  }
  const email = `integration-${Date.now()}@example.com`,
    password = "integration-browser-password";
  await post("/auth/register", { email, password, preferred_locale: "en" });
  await post("/auth/login", { email, password });
  const org = await post("/organizations", {
      name: "CRM Studio",
      country: "CZ",
    }),
    assistant = await post(`/organizations/${org.id}/assistants`, {
      name: "CRM guide",
    });
  let signingSecret = "",
    calls = 0;
  const events: {
    event_id: string;
    event_type: string;
    lead_id: string | null;
    created_at: string;
  }[] = [];
  const errors: string[] = [];
  const receiver = http.createServer(async (req, res) => {
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    const body = Buffer.concat(chunks),
      stamp = req.headers["x-siteassistant-timestamp"] as string,
      sig = req.headers["x-siteassistant-signature"] as string;
    const expected =
      "v1=" +
      createHmac("sha256", signingSecret)
        .update(stamp + ".")
        .update(body)
        .digest("hex");
    if (
      !sig ||
      sig.length !== expected.length ||
      !timingSafeEqual(Buffer.from(sig), Buffer.from(expected))
    )
      errors.push("signature");
    const event = JSON.parse(body.toString());
    if (
      Object.keys(event).sort().join(",") !==
      "created_at,event_id,event_type,lead_id"
    )
      errors.push("unexpected payload");
    events.push(event);
    if (++calls === 1) {
      res.writeHead(503).end();
      return;
    }
    res.writeHead(204).end();
  });
  await new Promise<void>((resolve, reject) => {
    receiver.once("error", reject);
    receiver.listen(18086, "127.0.0.1", resolve);
  });
  try {
    await page.goto("/");
    await page
      .getByRole("button")
      .filter({
        has: page.getByRole("heading", { name: "CRM Studio", exact: true }),
      })
      .click();
    await page
      .getByRole("button", { name: "Integrations", exact: true })
      .click();
    await page.getByLabel("Key name", { exact: true }).fill("Universal CRM");
    await page.getByLabel("Allow creating leads", { exact: true }).check();
    await page
      .getByRole("button", { name: "Create API key", exact: true })
      .click();
    const token = await page
      .getByLabel("New secret", { exact: true })
      .inputValue();
    expect(token).toMatch(/^sa_[A-Za-z0-9_-]{43}$/);
    await page
      .getByRole("button", { name: "Saved — hide secret", exact: true })
      .click();
    await expect(page.getByLabel("New secret", { exact: true })).toHaveCount(0);
    await page
      .getByLabel("Receiver URL", { exact: true })
      .fill("http://127.0.0.1:18086/webhook");
    await page
      .getByRole("button", { name: "Add webhook", exact: true })
      .click();
    signingSecret = await page
      .getByLabel("New secret", { exact: true })
      .inputValue();
    expect(signingSecret).toHaveLength(43);
    await page
      .getByRole("button", { name: "Saved — hide secret", exact: true })
      .click();
    await page.getByRole("button", { name: "Send test", exact: true }).click();
    await expect
      .poll(() => calls, { timeout: 20000 })
      .toBeGreaterThanOrEqual(2);
    expect(errors).toEqual([]);
    expect(events[0].event_id).toBe(events[1].event_id);
    expect(events[0].event_type).toBe("webhook.test");
    await expect(page.locator(".integration-deliveries")).toContainText(
      "Delivered",
      { timeout: 15000 },
    );
    const keyHeaders = {
        Authorization: "Bearer " + token,
        "Idempotency-Key": "browser-request-one",
      },
      input = {
        assistant_id: assistant.id,
        values: { full_name: "Anna", email: "anna@example.com" },
      };
    const r = await page.request.post("/api/v1/external/leads", {
      headers: keyHeaders,
      data: input,
    });
    expect(r.status(), await r.text()).toBe(201);
    const lead = await r.json();
    const again = await page.request.post("/api/v1/external/leads", {
      headers: keyHeaders,
      data: input,
    });
    expect((await again.json()).id).toBe(lead.id);
    await expect
      .poll(
        () => events.filter((e) => e.event_type === "lead.created").length,
        { timeout: 15000 },
      )
      .toBe(1);
    expect(events.find((e) => e.event_type === "lead.created")!.lead_id).toBe(
      lead.id,
    );
    expect(errors).toEqual([]);
    const feed = await (
      await page.request.get("/api/v1/external/leads?limit=1", {
        headers: { Authorization: "Bearer " + token },
      })
    ).json();
    expect(feed.items).toHaveLength(1);
    expect(feed.items[0].lead.email).toBe("anna@example.com");
    const empty = await (
      await page.request.get(
        "/api/v1/external/leads?cursor=" + feed.next_cursor,
        { headers: { Authorization: "Bearer " + token } },
      )
    ).json();
    expect(empty.items).toHaveLength(0);
    await page
      .getByRole("button", { name: "Attempt journal", exact: true })
      .first()
      .click();
    await expect(
      page.getByRole("region", { name: "Attempt journal", exact: true }),
    ).toContainText("Delivered");
    for (const [locale, title] of [
      ["ru", "Интеграции"],
      ["cs", "Integrace"],
      ["de", "Integrationen"],
      ["en", "Integrations"],
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
        path: `../artifacts/stage7/integrations-${width}.png`,
        fullPage: true,
      });
    }
    await page.reload();
    await page
      .getByRole("button")
      .filter({
        has: page.getByRole("heading", { name: "CRM Studio", exact: true }),
      })
      .click();
    await page
      .getByRole("button", { name: "Integrations", exact: true })
      .click();
    await expect(page.getByLabel("New secret", { exact: true })).toHaveCount(0);
    page.once("dialog", (d) => d.accept());
    await page.getByRole("button", { name: "Revoke", exact: true }).click();
    await expect(page.getByText("Revoked", { exact: true })).toBeVisible();
    expect(
      (
        await page.request.get("/api/v1/external/leads", {
          headers: { Authorization: "Bearer " + token },
        })
      ).status(),
    ).toBe(401);
    await page.getByRole("button", { name: "Leads", exact: true }).click();
    await page.locator(".lead-row").click();
    await expect(page.locator(".lead-detail")).toContainText("External API");
    await expect(
      page.getByRole("button", { name: "Open conversation", exact: true }),
    ).toHaveCount(0);
  } finally {
    await new Promise<void>((resolve) => receiver.close(() => resolve()));
  }
});
