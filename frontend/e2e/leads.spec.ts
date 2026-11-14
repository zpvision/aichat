import { test, expect } from "@playwright/test";
import fs from "node:fs";
test.use({ actionTimeout: 15000 });
test("scenario, partial mock extraction, confirmation and operator CRM with real PostgreSQL", async ({
  page,
  browser,
}) => {
  test.setTimeout(240000);
  fs.mkdirSync("../artifacts/stage6", { recursive: true });
  const headers = { Origin: "http://localhost:5173", "X-CSRF-Protection": "1" };
  const stamp = Date.now();
  async function post(path: string, data: unknown, client = page.request) {
    const r = await client.post("http://localhost:5173/api/v1" + path, {
      headers,
      data,
    });
    expect(r.ok(), await r.text()).toBe(true);
    return r.json();
  }
  async function account(client: typeof page.request, prefix: string) {
    const email = `${prefix}${stamp}@leads.example`,
      password = "lead-browser-password";
    await post(
      "/auth/register",
      { email, password, preferred_locale: "en" },
      client,
    );
    return post("/auth/login", { email, password }, client);
  }
  const owner = await account(page.request, "owner"),
    org = await post("/organizations", { name: "Lead Studio", country: "CZ" });
  const base = "/organizations/" + org.id;
  const assistant = await post(base + "/assistants", { name: "Request guide" });
  const site = await post(base + "/sites", {
    name: "Studio website",
    domain: "studio.example",
    assistant_id: assistant.id,
  });
  const admin = await post(base + "/sites/" + site.id + "/widget", {});
  const theme = admin.widget.settings;
  theme.privacy_url = "https://studio.example/privacy";
  expect(
    (
      await page.request.patch(
        "/api/v1" + base + "/sites/" + site.id + "/widget",
        {
          headers,
          data: {
            enabled: true,
            origins: ["http://localhost:4173"],
            settings: theme,
          },
        },
      )
    ).ok(),
  ).toBe(true);
  const operatorContext = await browser.newContext(),
    visitorContext = await browser.newContext();
  const operator = await account(operatorContext.request, "operator");
  const invite = await post(base + "/invitations", { role: "operator" });
  await post(
    "/invitations/accept",
    { token: invite.url.split("#invite=")[1] },
    operatorContext.request,
  );
  const visitor = await visitorContext.newPage(),
    op = await operatorContext.newPage();
  visitor.on("pageerror", (e) => console.log("WIDGET ERROR", e.message));
  try {
    await page.goto("/app");
    await page
      .getByRole("button")
      .filter({
        has: page.getByRole("heading", { name: "Lead Studio", exact: true }),
      })
      .click();
    await page.getByRole("button", { name: "Scenarios", exact: true }).click();
    await page.getByLabel("Collect visitor requests", { exact: true }).check();
    await page.getByLabel("Scenario name").fill("Consult and apply");
    await page
      .getByLabel("Collection goal")
      .fill("Collect a service request after visitor agreement.");
    await page.locator(".lead-field").first().locator("summary").click();
    await page
      .locator(".lead-field")
      .first()
      .getByLabel("Required", { exact: true })
      .check();
    await page.getByRole("button", { name: "Add field", exact: true }).click();
    const custom = page.locator(".lead-field").last();
    await custom.getByLabel("Stable key").fill("employee_count");
    await custom.getByLabel("Field label").fill("Employees");
    await custom.getByLabel("Type", { exact: true }).selectOption("number");
    await custom.getByLabel("Minimum", { exact: true }).fill("0");
    await custom.getByLabel("Maximum", { exact: true }).fill("10000");
    await custom.getByLabel("Required", { exact: true }).check();
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await expect(
      page.getByText("Changes saved", { exact: true }),
    ).toBeVisible();
    await visitor.goto(
      "http://localhost:4173/widget/demo/index.html?widget=" + admin.widget.id,
    );
    await visitor
      .getByRole("button", { name: "Open chat", exact: true })
      .click();
    const chat = visitor.frameLocator("iframe.sa-frame"),
      card = chat.getByRole("region", { name: "Request", exact: true });
    const input = chat.getByRole("textbox", {
      name: "Write your message…",
      exact: true,
    });
    await input.fill("Hello, what services do you provide?");
    await input.press("Enter");
    await expect(chat.locator(".message.assistant")).toContainText(
      "Demonstration response",
      { timeout: 45000 },
    );
    await expect(
      chat.getByRole("button", { name: "Stop response" }),
    ).toHaveCount(0);
    await card
      .getByRole("button", { name: "Start a request", exact: true })
      .click();
    await expect(
      card.getByText("Collecting details", { exact: true }),
    ).toBeVisible();
    await input.fill('{"full_name":"Anna","employee_count":12}');
    await input.press("Enter");
    await expect(card.getByText("Anna", { exact: true })).toBeVisible({
      timeout: 45000,
    });
    await expect(
      card.getByRole("button", { name: "Send request", exact: true }),
    ).toHaveCount(0);
    await expect(card).toContainText("Phone or email is sufficient");
    await input.fill('{"email":"anna@example.com"}');
    await input.press("Enter");
    await expect(
      card.getByRole("button", { name: "Send request", exact: true }),
    ).toBeVisible({ timeout: 45000 });
    // Change the assistant settings while collecting: active collection keeps its frozen fields.
    const settingPath = base + "/assistants/" + assistant.id + "/lead-settings",
      settings = await (await page.request.get("/api/v1" + settingPath)).json();
    settings.fields.find(
      (f: { key: string }) => f.key === "company_name",
    ).required = true;
    expect(
      (
        await page.request.put("/api/v1" + settingPath, {
          headers,
          data: settings,
        })
      ).ok(),
    ).toBe(true);
    await visitor.reload();
    await visitor
      .getByRole("button", { name: "Open chat", exact: true })
      .click();
    await expect(
      card.getByRole("button", { name: "Send request", exact: true }),
    ).toBeVisible({ timeout: 15000 });
    for (const width of [360, 768, 1440]) {
      await visitor.setViewportSize({ width, height: 900 });
      expect(
        await chat
          .locator("html")
          .evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      ).toBe(true);
      await visitor.screenshot({
        path: `../artifacts/stage6/confirmation-${width}.png`,
      });
    }
    await card
      .getByRole("button", { name: "Edit details", exact: true })
      .click();
    await card
      .getByLabel("Email", { exact: true })
      .fill("anna.updated@example.com");
    await card
      .getByRole("button", { name: "Save details", exact: true })
      .click();
    await expect(
      card.getByText("anna.updated@example.com", { exact: true }),
    ).toBeVisible();
    await card
      .getByRole("button", { name: "Send request", exact: true })
      .click();
    await expect(
      card.getByText("Thank you! Your request was sent to the company."),
    ).toBeVisible();
    await page.bringToFront();
    await page.getByRole("button", { name: "Leads", exact: true }).click();
    await expect(page.locator(".lead-row")).toHaveCount(1);
    await page.locator(".lead-row").click();
    await expect(page.locator(".lead-detail")).toContainText(
      "anna.updated@example.com",
    );
    await page
      .locator(".lead-detail")
      .getByLabel("Assignee", { exact: true })
      .selectOption(operator.id);
    await expect
      .poll(async () => {
        const r = await page.request.get("/api/v1" + base + "/leads");
        return (await r.json()).items[0].assigned_user_id;
      })
      .toBe(operator.id);
    const result = await (
      await page.request.get("/api/v1" + base + "/leads")
    ).json();
    const lead = result.items[0];
    expect(result.items).toHaveLength(1);
    // Lead assignment does not silently grant inbox permissions. Assign the conversation explicitly.
    const conversation = await (
      await page.request.get(
        "/api/v1" + base + "/conversations/" + lead.conversation_id,
      )
    ).json();
    await post(base + "/conversations/" + lead.conversation_id + "/assign", {
      request_id: crypto.randomUUID(),
      version: conversation.conversation.state_version,
      assignee: operator.id,
    });
    await op.goto("/app");
    await op
      .getByRole("button")
      .filter({
        has: op.getByRole("heading", { name: "Lead Studio", exact: true }),
      })
      .click();
    await op.getByRole("button", { name: "Leads", exact: true }).click();
    await op.locator(".lead-row").click();
    await op
      .locator(".lead-detail")
      .getByLabel("Status", { exact: true })
      .selectOption("in_progress");
    await op
      .getByLabel("Internal note", { exact: true })
      .fill("Call the visitor tomorrow. <script>window.bad=1</script>");
    await op.getByRole("button", { name: "Add note", exact: true }).click();
    await expect(op.locator(".lead-history")).toContainText(
      "Call the visitor tomorrow.",
    );
    expect(await op.evaluate(() => "bad" in window)).toBe(false);
    await op
      .getByRole("button", { name: "Open conversation", exact: true })
      .click();
    await expect(op.locator(".inbox-history")).toContainText("Anna");
    await expect(
      op.getByRole("button", { name: "Open lead", exact: true }),
    ).toBeVisible();
    await op.getByRole("button", { name: "Open lead", exact: true }).click();
    await op.reload();
    await op
      .getByRole("button")
      .filter({
        has: op.getByRole("heading", { name: "Lead Studio", exact: true }),
      })
      .click();
    await op.getByRole("button", { name: "Leads", exact: true }).click();
    await op.locator(".lead-row").click();
    await expect(
      op.locator(".lead-detail").getByLabel("Status", { exact: true }),
    ).toHaveValue("in_progress");
    await expect(op.locator(".lead-history")).toContainText(
      "Call the visitor tomorrow.",
    );
    for (const [locale, title] of [
      ["ru", "Лиды"],
      ["cs", "Zájemci"],
      ["de", "Leads"],
      ["en", "Leads"],
    ]) {
      await op.locator("header select").first().selectOption(locale);
      await expect(
        op.getByRole("heading", { name: title, exact: true }),
      ).toBeVisible();
    }
    for (const width of [360, 768, 1440]) {
      await op.setViewportSize({ width, height: 960 });
      expect(
        await op
          .locator("html")
          .evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      ).toBe(true);
      await op.screenshot({
        path: `../artifacts/stage6/crm-${width}.png`,
        fullPage: true,
      });
    }
    await visitor.bringToFront();
    for (const locale of ["ru", "en", "cs", "de"]) {
      await chat.getByRole("combobox").selectOption(locale);
      await expect(chat.locator(".lead-card")).toBeVisible();
    }
    const publicNote = await chat.locator("body").innerText();
    expect(publicNote).not.toContain("Call the visitor tomorrow");
  } finally {
    await operatorContext.close();
    await visitorContext.close();
  }
});
