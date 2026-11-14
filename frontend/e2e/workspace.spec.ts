import { test, expect } from "@playwright/test";
test("registration, company, assistant, site, locale, logout and login", async ({
  page,
}) => {
  const email = `browser-${Date.now()}@example.com`;
  const password = "browser-password-1234";
  await page.goto("/login");
  await page
    .getByRole("combobox", { name: "Interface language" })
    .selectOption("en");
  await page
    .getByRole("button", { name: "New here? Create an account" })
    .click();
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByLabel("Repeat password", { exact: true }).fill(password);
  await page
    .getByRole("button", { name: "Create account", exact: true })
    .click();
  await page.getByRole("button", { name: "Sign in", exact: true }).waitFor();
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page
    .getByRole("button", { name: "Create company", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Company name").fill("Browser Company");
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await page
    .getByRole("button", { name: "AI employees", exact: true })
    .first()
    .click();
  await page
    .getByRole("button", { name: "Create assistant", exact: true })
    .click();
  await dialog.getByLabel("Name", { exact: true }).fill("Website guide");
  await dialog.getByLabel("Description").fill("Company support configuration");
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Website guide" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Websites", exact: true }).click();
  await page.getByRole("button", { name: "Add website", exact: true }).click();
  await dialog.getByLabel("Name", { exact: true }).fill("Company website");
  await dialog.getByLabel("Domain", { exact: true }).fill("EXAMPLE.COM");
  await dialog
    .getByRole("combobox", { name: "Assistant", exact: true })
    .selectOption({ label: "Website guide" });
  await dialog.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByText("example.com", { exact: true })).toBeVisible();
  for (const [locale, heading] of [
    ["ru", "Сайты"],
    ["cs", "Weby"],
    ["de", "Websites"],
    ["en", "Websites"],
  ]) {
    await page.locator("header select").selectOption(locale);
    await expect(page.locator("main h1")).toHaveText(heading);
  }
  for (const width of [360, 768, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    await expect(page.locator("main h1")).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: `test-results/workspace-${width}.png`,
      fullPage: true,
    });
  }
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Cancel" })
    .click();
  await expect(page.getByText("example.com", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Delete", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "No websites connected yet" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Profile", exact: true }).click();
  await page.getByLabel("Name", { exact: true }).fill("Browser User");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Changes saved");
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Browser Company" }),
  ).toBeVisible();
});

test("authentication forms validate and display API errors on mobile", async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 900 });
  await page.goto("/login");
  await page.locator("header select").selectOption("en");
  await page.getByLabel("Email", { exact: true }).fill("unknown@example.com");
  await page
    .getByLabel("Password", { exact: true })
    .fill("wrong-password-1234");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Invalid email or password.",
  );
  await page
    .getByRole("button", { name: "New here? Create an account" })
    .click();
  await page.getByLabel("Email", { exact: true }).fill("form@example.com");
  await page
    .getByLabel("Password", { exact: true })
    .fill("first-password-1234");
  await page
    .getByLabel("Repeat password", { exact: true })
    .fill("other-password-1234");
  await page
    .getByRole("button", { name: "Create account", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText(
    "Passwords do not match.",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
