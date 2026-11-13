import { test, expect } from "@playwright/test";
test("installed widget: appearance, anonymous sessions, streaming, cancellation and isolation", async ({
  page,
}) => {
  test.setTimeout(180000);
  const headers = { Origin: "http://localhost:5173", "X-CSRF-Protection": "1" };
  const post = async (path: string, data: unknown) => {
    const r = await page.request.post("/api/v1" + path, { headers, data });
    expect(r.ok(), await r.text()).toBe(true);
    return r.json();
  };
  const email = `widget-${Date.now()}@example.com`,
    password = "widget-browser-password";
  await post("/auth/register", { email, password, preferred_locale: "en" });
  await post("/auth/login", { email, password });
  const org = await post("/organizations", {
    name: "Widget Browser",
    country: "CZ",
  });
  const assistant = await post("/organizations/" + org.id + "/assistants", {
    name: "Northline guide",
  });
  const site = await post("/organizations/" + org.id + "/sites", {
    name: "Northline site",
    domain: "northline.example",
    assistant_id: assistant.id,
  });
  const admin = "/organizations/" + org.id + "/sites/" + site.id + "/widget";
  const created = await post(admin, {});
  const widgetId = created.widget.id;
  const asst = "/organizations/" + org.id + "/assistants/" + assistant.id;
  const source = await post(asst + "/knowledge/articles", {
    title: "Northline services",
    content:
      "Northline offers accounting support for companies. The Aurora plan costs 450 EUR per month and includes tax reporting and payroll services.",
    language: "en",
  });
  await expect
    .poll(
      async () => {
        const r = await page.request.get(
          "/api/v1" + asst + "/knowledge/" + source.id,
        );
        return (await r.json()).status;
      },
      { timeout: 45000 },
    )
    .toBe("ready");
  await page.goto("/");
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", { name: "Widget Browser", exact: true }),
    })
    .click();
  await page.getByRole("button", { name: "Website chat", exact: true }).click();
  await page
    .getByLabel("Consultant name", { exact: true })
    .fill("Northline assistant");
  await page
    .getByLabel("Short subtitle", { exact: true })
    .fill("Clear answers. A little closer.");
  await page
    .getByLabel("Greeting", { exact: true })
    .fill(
      "Hello! 👋 Let’s find the right information for you. What would you like to know?",
    );
  const preview = page.frameLocator('iframe[title="Live appearance preview"]');
  await expect(
    preview.getByRole("heading", { name: "Northline assistant" }),
  ).toBeVisible();
  await page
    .getByLabel("Allowed origins, one per line")
    .fill("http://localhost:4173");
  await page.getByLabel("Enable the widget on this website").check();
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Changes saved");
  const demo =
    "http://localhost:4173/widget/demo/index.html?widget=" + widgetId;
  await page.goto(demo);
  const launcher = page.getByRole("button", { name: "Open chat", exact: true });
  await expect(launcher).toBeVisible();
  expect(await page.locator("iframe.sa-frame").count()).toBe(0);
  const bodyColor = await page
    .locator("body")
    .evaluate((el) => getComputedStyle(el).backgroundColor);
  await launcher.click();
  const chat = page.frameLocator("iframe.sa-frame");
  await expect(
    chat.getByRole("heading", { name: "Northline assistant" }),
  ).toBeVisible();
  await expect(chat.getByText(/Demo mode/)).toBeVisible();
  await page.screenshot({ path: "test-results/widget-welcome.png" });
  await expect(
    chat.getByRole("textbox", { name: "Write your message…" }),
  ).toBeEnabled();
  expect(
    await page
      .locator("body")
      .evaluate((el) => getComputedStyle(el).backgroundColor),
  ).toBe(bodyColor);
  await chat
    .getByRole("textbox", { name: "Write your message…" })
    .fill("Wie viel kostet der Aurora Tarif?");
  await chat
    .getByRole("textbox", { name: "Write your message…" })
    .press("Enter");
  await expect(chat.locator(".message.assistant")).toContainText(
    "Demonstrationsantwort",
    { timeout: 45000 },
  );
  await expect(chat.getByRole("button", { name: "Stop response" })).toHaveCount(
    0,
    { timeout: 45000 },
  );
  await expect(chat.locator(".message.user")).toHaveCount(1);
  const saved = await page.evaluate(
    (id) => sessionStorage.getItem("site-assistant:" + id),
    widgetId,
  );
  expect(saved).toBeTruthy();
  const session = JSON.parse(saved!);
  expect(session.token).toMatch(/^[A-Za-z0-9_-]{43}$/);
  expect(page.url()).not.toContain(session.token);
  for (const width of [360, 768, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(page.locator("iframe.sa-frame")).toBeVisible();
    expect(
      await chat
        .locator("html")
        .evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    ).toBe(true);
    await page.screenshot({
      path: `test-results/widget-${width}.png`,
      fullPage: false,
    });
  }
  await page.evaluate(
    ({ channel }) =>
      window.postMessage({ type: "close", channel }, location.origin),
    {
      channel: new URL(
        (await page.locator("iframe.sa-frame").getAttribute("src")) as string,
      ).searchParams.get("channel"),
    },
  );
  await expect(page.locator("iframe.sa-frame")).toBeVisible();
  await chat.getByRole("button", { name: "Close chat", exact: true }).click();
  await expect(page.locator("iframe.sa-frame")).toBeHidden();
  await launcher.click();
  await expect(chat.locator(".message.assistant")).toHaveCount(1);
  await page.reload();
  await page.getByRole("button", { name: "Open chat", exact: true }).click();
  await expect(chat.locator(".message.assistant")).toHaveCount(1);
  expect(
    JSON.parse(
      (await page.evaluate(
        (id) => sessionStorage.getItem("site-assistant:" + id),
        widgetId,
      )) as string,
    ).id,
  ).toBe(session.id);
  await chat
    .getByRole("textbox", { name: "Write your message…" })
    .fill("What is included?");
  await chat.getByRole("button", { name: "Send message" }).click();
  await chat.getByRole("button", { name: "Stop response" }).click();
  await expect(
    chat.getByRole("button", { name: "Send message" }),
  ).toBeVisible();
  await expect(chat.getByRole("alert")).toContainText("Response stopped.");
  for (const [locale, placeholder] of [
    ["ru", "Напишите сообщение…"],
    ["cs", "Napište zprávu…"],
    ["de", "Ihre Nachricht…"],
    ["en", "Write your message…"],
  ]) {
    await chat.getByRole("combobox").selectOption(locale);
    await expect(
      chat.getByRole("textbox", { name: placeholder }),
    ).toBeVisible();
  }

  await page.setViewportSize({ width: 360, height: 480 });
  await expect(
    chat.getByRole("textbox", { name: "Write your message…" }),
  ).toBeVisible();
  await expect
    .poll(
      async () =>
        await page
          .locator("iframe.sa-frame")
          .evaluate((el) => el.getBoundingClientRect().height),
    )
    .toBeLessThanOrEqual(480);
  await chat
    .getByRole("button", { name: "New conversation", exact: true })
    .click();
  await chat
    .getByRole("alertdialog")
    .getByRole("button", { name: "Keep conversation", exact: true })
    .press("Tab");
  await expect(
    chat
      .getByRole("alertdialog")
      .getByRole("button", { name: "Start new", exact: true }),
  ).toBeFocused();
  await chat
    .getByRole("alertdialog")
    .getByRole("button", { name: "Start new", exact: true })
    .click();
  await expect(chat.locator(".message")).toHaveCount(0);
  await chat
    .getByRole("textbox", { name: "Write your message…" })
    .fill(
      '<img src=x onerror="window.pwned=true"> Hello ' +
        "Longword".repeat(30) +
        " https://example.com/" +
        "longurl".repeat(30),
    );
  await chat
    .getByRole("textbox", { name: "Write your message…" })
    .press("Enter");
  await expect(chat.getByRole("button", { name: "Stop response" })).toHaveCount(
    0,
    { timeout: 45000 },
  );
  await expect(chat.locator(".message.user")).toContainText("<img src=x");
  expect(await chat.locator(".message.user img").count()).toBe(0);
  expect(
    await chat
      .locator("html")
      .evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBe(true);
  await chat.locator("html").evaluate((el) => (el.style.fontSize = "32px"));
  await expect(
    chat.getByRole("textbox", { name: "Write your message…" }),
  ).toBeInViewport();
  expect(
    await chat
      .locator("html")
      .evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBe(true);
  await chat.locator("html").evaluate((el) => (el.style.fontSize = "16px"));
  const otherSession = JSON.parse(
    (await page.evaluate(
      (id) => sessionStorage.getItem("site-assistant:" + id),
      widgetId,
    )) as string,
  );
  expect(otherSession.id).not.toBe(session.id);
  const mismatch = await page.request.get(
    "http://localhost:8080/api/v1/public/widgets/" +
      widgetId +
      "/sessions/" +
      otherSession.id +
      "/messages",
    {
      headers: {
        Origin: "http://localhost:4173",
        Authorization: "Bearer " + session.token,
      },
    },
  );
  expect(mismatch.status()).toBe(401);
  const disabled = await page.request.patch(
    "http://localhost:5173/api/v1" + admin,
    { headers, data: { enabled: false } },
  );
  expect(disabled.ok()).toBe(true);
  await chat
    .getByRole("textbox", { name: "Write your message…" })
    .fill("Hello again");
  await chat.getByRole("button", { name: "Send message" }).click();
  await expect(chat.getByRole("alert")).toContainText("unavailable");
});
