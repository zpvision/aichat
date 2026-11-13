import { test, expect } from "@playwright/test";
test("preview chat uses real RAG and explicit mock generation with history and cancellation", async ({
  page,
}) => {
  test.setTimeout(120000);
  const headers = { Origin: "http://localhost:5173", "X-CSRF-Protection": "1" };
  const post = async (path: string, data: unknown) => {
    const r = await page.request.post("/api/v1" + path, { headers, data });
    expect(r.ok(), await r.text()).toBe(true);
    return r.json();
  };
  const email = `preview-${Date.now()}@example.com`,
    password = "preview-test-password";
  await post("/auth/register", { email, password, preferred_locale: "en" });
  await post("/auth/login", { email, password });
  const org = await post("/organizations", {
    name: "Preview Company",
    country: "CZ",
  });
  const assistant = await post("/organizations/" + org.id + "/assistants", {
    name: "Preview guide",
  });
  const base = "/organizations/" + org.id + "/assistants/" + assistant.id;
  const source = await post(base + "/knowledge/articles", {
    title: "Aurora knowledge",
    content:
      "Тариф Aurora включает бухгалтерское сопровождение, расчёт зарплаты и подготовку налоговых отчётов. Цена составляет 450 евро в месяц.",
    language: "ru",
  });
  await expect
    .poll(
      async () => {
        const r = await page.request.get(
          "/api/v1" + base + "/knowledge/" + source.id,
        );
        return (await r.json()).status;
      },
      { timeout: 45000 },
    )
    .toBe("ready");
  const open = async () => {
    await page
      .getByRole("button")
      .filter({
        has: page.getByRole("heading", {
          name: "Preview Company",
          exact: true,
        }),
      })
      .click();
    await page.getByRole("button", { name: "Test chat", exact: true }).click();
  };
  await page.goto("/");
  await open();
  await expect(
    page.getByText(/Demonstration mode — responses are test responses/),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Assistant instructions", exact: true })
    .click();
  await page
    .getByLabel("Assistant role", { exact: true })
    .fill("Support specialist");
  await page
    .getByLabel("Company description", { exact: true })
    .fill("Aurora accounting services");
  await page
    .getByLabel("Communication style", { exact: true })
    .fill("Clear and concise");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await page
    .getByRole("button", { name: "New conversation", exact: true })
    .click();
  const message = page.getByRole("textbox", { name: "Message", exact: true });
  await message.fill("Wie viel kostet der Tarif Aurora?");
  await message.press("Shift+Enter");
  await expect(message).toHaveValue("Wie viel kostet der Tarif Aurora?\n");
  await message.press("Enter");
  await expect(page.getByRole('button',{name:'Stop generation'})).toBeVisible();
  await expect(page.locator(".chat-message.assistant")).toHaveCount(1);
  await expect(page.locator('.chat-message.assistant')).toContainText('Demonstrationsantwort',{timeout:45000});
  await expect(
    page.getByRole("button", { name: "Stop generation" }),
  ).toHaveCount(0, { timeout: 45000 });
  await expect(page.locator(".chat-message.assistant")).toContainText(
    "Demonstrationsantwort",
  );
  await page.getByText("Sources passed to AI (1)", { exact: true }).click();
  await expect(
    page.getByText("Aurora knowledge", { exact: true }),
  ).toBeVisible();
  await page.locator("header select").selectOption("ru");
  const ruMessage = page.getByRole("textbox", {
    name: "Сообщение",
    exact: true,
  });
  await ruMessage.fill("What is included in Aurora?");
  await ruMessage.press("Enter");
  await expect(
    page.getByRole("button", { name: "Остановить генерацию" }),
  ).toHaveCount(0, { timeout: 45000 });
  await expect(page.locator(".chat-message.assistant")).toHaveCount(2);
  await expect(page.locator(".chat-message.assistant").last()).toContainText(
    "Demonstration response",
  );
  await page.locator("header select").selectOption("en");
  await page.reload();
  await open();
  await expect(page.locator(".chat-message.assistant")).toHaveCount(2);
  await expect(page.locator(".chat-message.user")).toHaveCount(2);
  const streamStarted = page.waitForResponse((r) =>
    r.url().endsWith("/messages/stream"),
  );
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .fill("What is the price again?");
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .press("Enter");
  await streamStarted;
  await page
    .getByRole("button", { name: "Stop generation", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeVisible();
  await expect
    .poll(async () => {
      const list = await (
        await page.request.get("/api/v1" + base + "/preview/conversations")
      ).json();
      const d = await (
        await page.request.get(
          "/api/v1" + base + "/preview/conversations/" + list[0].id,
        )
      ).json();
      return d.executions.some(
        (e: { status: string }) => e.status === "processing",
      );
    })
    .toBe(false);
  const historyList = await (
    await page.request.get("/api/v1" + base + "/preview/conversations")
  ).json();
  const canceledHistory = await (
    await page.request.get(
      "/api/v1" + base + "/preview/conversations/" + historyList[0].id,
    )
  ).json();
  expect(
    canceledHistory.executions.some(
      (e: { status: string; error_code: string }) =>
        e.status === "canceled" && e.error_code === "ai_canceled",
    ),
  ).toBe(true);
  await expect(page.getByRole("alert")).toContainText(
    "Generation was canceled.",
  );
  for (const locale of ["ru", "cs", "de", "en"]) {
    await page.locator("header select").selectOption(locale);
    await expect(page.locator("html")).toHaveAttribute("lang", locale);
  }
  for (const width of [360, 768, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `test-results/preview-${width}.png`,
      fullPage: true,
    });
  }
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Delete", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Start a test conversation" }),
  ).toBeVisible();
});
