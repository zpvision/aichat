import { test, expect } from "@playwright/test";
test("knowledge articles, upload, real retrieval and responsive locales", async ({
  page,
}) => {
  test.setTimeout(120000);
  const email = `knowledge-browser-${Date.now()}@example.com`,
    password = "knowledge-browser-password";
  const headers = { Origin: "http://localhost:5173", "X-CSRF-Protection": "1" };
  const post = async (path: string, data: unknown) => {
    const r = await page.request.post("/api/v1" + path, { headers, data });
    expect(r.ok(), await r.text()).toBe(true);
    return r.json();
  };
  await post("/auth/register", { email, password, preferred_locale: "en" });
  await post("/auth/login", { email, password });
  const org = await post("/organizations", {
    name: "Knowledge Browser",
    country: "CZ",
  });
  await post("/organizations/" + org.id + "/assistants", {
    name: "Knowledge guide",
  });
  await page.goto("/");
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", {
        name: "Knowledge Browser",
        exact: true,
      }),
    })
    .click();
  await page
    .getByRole("button", { name: "Knowledge base", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Create article", exact: true })
    .click();
  await page.getByLabel("Material title").fill("Tariff Aurora");
  await page
    .getByLabel("Content", { exact: true })
    .fill(
      "Тариф Aurora включает бухгалтерское сопровождение, подготовку налоговой отчётности и расчёт зарплаты. Стоимость тарифа составляет 450 евро в месяц.",
    );
  await page
    .getByRole("combobox", { name: "Document language" })
    .selectOption("ru");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  const article = page.locator("article").filter({
    has: page.getByRole("heading", { name: "Tariff Aurora", exact: true }),
  });
  await expect(article.getByText("Ready", { exact: true })).toBeVisible({
    timeout: 45000,
  });
  await article.getByRole("button", { name: "Edit", exact: true }).click();
  await page
    .getByLabel("Content", { exact: true })
    .fill(
      "Тариф Aurora включает бухгалтерское сопровождение и расчёт зарплаты. Стоимость тарифа составляет 500 евро в месяц.",
    );
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(article.getByText("Ready", { exact: true })).toBeVisible({
    timeout: 45000,
  });
  await page.getByRole("tab", { name: "Documents", exact: true }).click();
  await page.getByLabel("Choose a document").setInputFiles({
    name: "contacts.txt",
    mimeType: "text/plain",
    buffer: Buffer.from(
      "Contact office: Monday to Friday. Email help@example.com. Český text a русский текст.",
    ),
  });
  await page.getByRole("button", { name: "Upload", exact: true }).click();
  await expect(page.getByText("Ready", { exact: true })).toBeVisible({
    timeout: 45000,
  });
  await page.getByRole("tab", { name: "Test knowledge", exact: true }).click();
  await page.getByRole("tab", { name: "Website import", exact: true }).click();
  await page.getByLabel("Page URL").fill("https://example.com/");
  await page.getByRole("button", { name: "Import", exact: true }).click();
  await expect(
    page
      .locator("article")
      .filter({
        has: page.getByRole("heading", { name: "Example Domain", exact: true }),
      })
      .getByText("Ready", { exact: true }),
  ).toBeVisible({ timeout: 45000 });
  await page.getByRole("tab", { name: "Test knowledge", exact: true }).click();
  await page
    .getByLabel("Your question")
    .fill("Wie viel kostet der Tarif Aurora pro Monat?");
  await page
    .getByRole("button", { name: "Find passages", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Tariff Aurora", exact: true }),
  ).toBeVisible({ timeout: 45000 });
  await expect(page.getByText(/500 евро в месяц/)).toBeVisible();
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
      path: `test-results/knowledge-${width}.png`,
      fullPage: true,
    });
  }
  await page.getByRole("tab", { name: "Articles", exact: true }).click();
  await article.getByRole("button", { name: "Delete", exact: true }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Delete", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "No materials yet" }),
  ).toBeVisible();
});
