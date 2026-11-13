import { test, expect } from "@playwright/test";
test.use({ actionTimeout: 15000 });
test("operator inbox, invitations and website handoff end to end", async ({
  page,
  browser,
}) => {
  test.setTimeout(180000);
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
    const email = prefix + stamp + "@inbox.example",
      password = "inbox-browser-password";
    await post(
      "/auth/register",
      { email, password, preferred_locale: "en" },
      client,
    );
    return post("/auth/login", { email, password }, client);
  }
  const owner = await account(page.request, "owner");
  const org = await post("/organizations", {
    name: "Operator Studio",
    country: "CZ",
  });
  const assistant = await post("/organizations/" + org.id + "/assistants", {
    name: "Studio guide",
  });
  const site = await post("/organizations/" + org.id + "/sites", {
    name: "Studio website",
    domain: "studio.example",
    assistant_id: assistant.id,
  });
  const settings = "/organizations/" + org.id + "/sites/" + site.id + "/widget";
  const admin = await post(settings, {});
  const enabled = await page.request.patch("/api/v1" + settings, {
    headers,
    data: { enabled: true, origins: ["http://localhost:4173"] },
  });
  expect(enabled.ok()).toBe(true);
  const one = await browser.newContext(),
    two = await browser.newContext(),
    visitorContext = await browser.newContext();
  const op1 = await account(one.request, "operator-one"),
    op2 = await account(two.request, "operator-two");
  const p1 = await one.newPage(),
    p2 = await two.newPage(),
    visitor = await visitorContext.newPage();
  try {
    await page.goto("/");
    await page
      .getByRole("button")
      .filter({
        has: page.getByRole("heading", {
          name: "Operator Studio",
          exact: true,
        }),
      })
      .click();
    await page.getByRole("button", { name: "Team", exact: true }).click();
    await page
      .getByRole("button", { name: "Create invitation", exact: true })
      .click();
    const link = await page
      .getByLabel("Invitation link", { exact: true })
      .inputValue();
    expect(link).toContain("#invite=");
    await p1.goto(link);
    await p1
      .getByRole("button", { name: "Accept invitation", exact: true })
      .click();
    await p1
      .getByRole("button")
      .filter({
        has: p1.getByRole("heading", { name: "Operator Studio", exact: true }),
      })
      .click();
    const secondInvite = await post(
      "/organizations/" + org.id + "/invitations",
      { role: "operator" },
    );
    await p2.goto(secondInvite.url);
    await p2
      .getByRole("button", { name: "Accept invitation", exact: true })
      .click();
    await p2
      .getByRole("button")
      .filter({
        has: p2.getByRole("heading", { name: "Operator Studio", exact: true }),
      })
      .click();
    await visitor.goto(
      "http://localhost:4173/widget/demo/index.html?widget=" + admin.widget.id,
    );
    await visitor
      .getByRole("button", { name: "Open chat", exact: true })
      .click();
    const chat = visitor.frameLocator("iframe.sa-frame");
    const input = chat.getByRole("textbox", { name: "Write your message…" });
    await input.fill("Hello, I need help with service terms.");
    await input.press("Enter");
    await expect(chat.locator(".message.assistant")).toContainText(
      "Demonstration response",
      { timeout: 45000 },
    );
    await expect(
      chat.getByRole("button", { name: "Stop response" }),
    ).toHaveCount(0, { timeout: 45000 });
    await chat
      .getByRole("button", { name: "Contact a person", exact: true })
      .click();
    await expect(
      chat.getByText(/Your request has been passed/).first(),
    ).toBeVisible();
    await page.bringToFront();
    await expect(page.getByText(/Staff requests: 1/)).toBeVisible({
      timeout: 10000,
    });
    for (const p of [page, p1, p2]) {
      await p.bringToFront();
      await p
        .getByRole("button", { name: "Conversations", exact: true })
        .click();
    }
    await page.bringToFront();
    await page
      .getByLabel("Mode", { exact: true })
      .selectOption("human_requested");
    await expect(page.locator(".conversation-row")).toHaveCount(1, {
      timeout: 10000,
    });
    await page.locator(".conversation-row").click();
    await p1.bringToFront();
    await p1.locator(".conversation-row").click();
    await p1
      .getByRole("button", { name: "Take conversation", exact: true })
      .click();
    await p1
      .getByLabel("Staff reply", { exact: true })
      .fill("Our colleague can help with your service terms.");
    await p1.getByRole("button", { name: "Send", exact: true }).click();
    await visitor.bringToFront();
    await expect(chat.locator(".message.operator")).toContainText(
      "Our colleague can help",
      { timeout: 10000 },
    );
    await expect(chat.locator(".virtual")).toHaveText("Company staff");
    await visitor.reload();
    await visitor
      .getByRole("button", { name: "Open chat", exact: true })
      .click();
    await visitor.bringToFront();
    await expect(chat.locator(".message.operator")).toContainText(
      "Our colleague can help",
      { timeout: 10000 },
    );
    await expect(chat.locator(".virtual")).toHaveText("Company staff");
    await input.fill("A visitor follow-up for the colleague.");
    await input.press("Enter");
    await expect(
      chat.getByRole("button", { name: "Stop response" }),
    ).toHaveCount(0, { timeout: 10000 });
    await p1.bringToFront();
    await expect(p1.locator(".inbox-message.user").last()).toContainText(
      "A visitor follow-up",
      { timeout: 10000 },
    );
    await expect(chat.locator(".message.assistant")).toHaveCount(1);
    await p1
      .getByLabel("Add note", { exact: true })
      .fill("Internal-only note, never show visitor");
    await p1.getByRole("button", { name: "Add note", exact: true }).click();
    await expect(p1.locator(".internal-note")).toContainText(
      "Internal-only note",
      { timeout: 10000 },
    );
    expect(
      await chat.getByText("Internal-only note, never show visitor").count(),
    ).toBe(0);
    await p2.bringToFront();
    await expect(p2.locator(".conversation-row")).toHaveCount(0, {
      timeout: 10000,
    });
    await page.bringToFront();
    await expect(page.locator(".thread-header")).toContainText(
      "Staff handling",
      { timeout: 10000 },
    );
    await page.getByLabel("Assign staff", { exact: true }).selectOption(op2.id);
    await page
      .getByRole("button", { name: "Assign staff", exact: true })
      .click();
    await p1.bringToFront();
    await expect(p1.getByLabel("Staff reply", { exact: true })).toHaveCount(0, {
      timeout: 10000,
    });
    await p2.bringToFront();
    await expect(p2.locator(".conversation-row")).toHaveCount(1, {
      timeout: 10000,
    });
    await p2.locator(".conversation-row").click();
    await p2
      .getByLabel("Staff reply", { exact: true })
      .fill("A second colleague is continuing this conversation.");
    await p2.getByRole("button", { name: "Send", exact: true }).click();
    await visitor.bringToFront();
    await expect(chat.locator(".message.operator").last()).toContainText(
      "A second colleague",
      { timeout: 10000 },
    );
    await p2.bringToFront();
    await p2.getByRole("button", { name: "Return to AI", exact: true }).click();
    await visitor.bringToFront();
    await expect(chat.locator(".virtual")).toHaveText("AI assistant", {
      timeout: 10000,
    });
    await input.fill("Thank you. What information do you have?");
    await input.press("Enter");
    await expect(chat.locator(".message.assistant").last()).toContainText(
      "Demonstration response",
      { timeout: 45000 },
    );
    await expect(
      chat.getByRole("button", { name: "Stop response" }),
    ).toHaveCount(0, { timeout: 45000 });
    await page.bringToFront();
    await expect(page.locator(".thread-header")).toContainText("AI handling", {
      timeout: 10000,
    });
    await page
      .getByRole("button", { name: "Close conversation", exact: true })
      .click();
    await page
      .getByRole("alertdialog")
      .getByRole("button", { name: "Close conversation", exact: true })
      .click();
    await visitor.bringToFront();
    await expect(
      chat.getByText(/This conversation is closed/).first(),
    ).toBeVisible({ timeout: 10000 });
    await expect(input).toBeDisabled();
    await page.bringToFront();
    await expect(page.locator(".inbox-message.assistant").last()).toContainText(
      "Demonstration response",
      { timeout: 10000 },
    );
    const ordered = page.locator(".inbox-message");
    await expect(ordered.last()).toHaveClass(/assistant/);
    await expect(ordered.nth((await ordered.count()) - 2)).toHaveClass(/user/);
    await expect
      .poll(
        async () => {
          const r = await page.request.get(
            "/api/v1/organizations/" + org.id + "/inbox-summary",
          );
          return (await r.json()).unread;
        },
        { timeout: 10000 },
      )
      .toBe(0);
    await page.getByLabel("Mode", { exact: true }).selectOption("closed");
    await page
      .getByLabel("Search messages", { exact: true })
      .fill("second colleague");
    await expect(page.locator(".conversation-row")).toHaveCount(1, {
      timeout: 10000,
    });
    for (const [locale, title] of [
      ["ru", "Диалоги"],
      ["cs", "Konverzace"],
      ["de", "Gespräche"],
      ["en", "Conversations"],
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
        path: `test-results/inbox-${width}.png`,
        fullPage: true,
      });
    }
    const list = await page.request.get(
      "/api/v1/organizations/" + org.id + "/conversations",
    );
    expect((await list.json()).items.length).toBe(1);
    const id = (
      await (
        await page.request.get(
          "/api/v1/organizations/" + org.id + "/conversations",
        )
      ).json()
    ).items[0].id;
    const denied = await visitorContext.request.get(
      "http://localhost:5173/api/v1/organizations/" +
        org.id +
        "/conversations/" +
        id,
    );
    expect(denied.status()).toBe(401);
    // A long manual conversation survives reload and exposes older pages in the actual iframe.
    await visitor.bringToFront();
    await chat
      .getByRole("button", { name: "New conversation", exact: true })
      .click();
    await chat
      .getByRole("alertdialog")
      .getByRole("button", { name: "Start new", exact: true })
      .click();
    await chat
      .getByRole("button", { name: "Contact a person", exact: true })
      .click();
    await expect(
      chat.getByText(/Your request has been passed/).first(),
    ).toBeVisible();
    const queue = await (
      await page.request.get(
        "/api/v1/organizations/" +
          org.id +
          "/conversations?mode=human_requested",
      )
    ).json();
    const fresh = queue.items[0];
    const item = "/organizations/" + org.id + "/conversations/" + fresh.id;
    const taken = await post(item + "/take", {
      request_id: crypto.randomUUID(),
      version: fresh.state_version,
    });
    for (let i = 0; i < 55; i++)
      await post(item + "/messages", {
        request_id: crypto.randomUUID(),
        version: taken.state_version,
        content:
          i === 0 ? "History beginning message" : "History continuation " + i,
      });
    await visitor.reload();
    await visitor
      .getByRole("button", { name: "Open chat", exact: true })
      .click();
    await chat
      .getByRole("button", { name: "Load earlier messages", exact: true })
      .click();
    await expect(
      chat.getByText("History beginning message", { exact: true }),
    ).toBeVisible();
  } finally {
    await Promise.allSettled([
      one.close(),
      two.close(),
      visitorContext.close(),
    ]);
  }
});
