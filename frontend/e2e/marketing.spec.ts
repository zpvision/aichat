import { test, expect } from "@playwright/test";
import { mkdir } from "node:fs/promises";
import en from "../src/locales/en/marketing.json" with { type: "json" };
import ru from "../src/locales/ru/marketing.json" with { type: "json" };
import cs from "../src/locales/cs/marketing.json" with { type: "json" };
import de from "../src/locales/de/marketing.json" with { type: "json" };
const dir = "../artifacts/stage9a2";
test("Cosmic Violet: four languages, six widths, complete page and isolated design", async ({
  page,
}) => {
  test.setTimeout(180000);
  await mkdir(dir, { recursive: true });
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => {
    if (m.type() === "error") errors.push(m.text());
  });
  await page.emulateMedia({ reducedMotion: "reduce" });
  for (const [lang, dict] of Object.entries({ ru, en, cs, de })) {
    await page.goto("/?lang=" + lang);
    await expect(page.locator("h1")).toHaveText(dict.copy.hero.join(""));
    await page.evaluate(() => document.fonts.ready);
    for (const width of [360, 390, 768, 1024, 1440, 1920]) {
      await page.setViewportSize({ width, height: 1000 });
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      expect(
        await page
          .locator("h1")
          .evaluate((e) => e.scrollWidth <= e.clientWidth + 1),
      ).toBe(true);
      await page.screenshot({
        path: `${dir}/iteration2-hero-${lang}-${width}.png`,
      });
    }
    expect(await page.locator(".cosmic main>section").count()).toBe(13);
    await expect(page.locator("html")).toHaveAttribute("lang", lang);
    await expect(page).toHaveTitle(dict.copy.title);
    await expect(page.locator("meta[name=description]")).toHaveAttribute(
      "content",
      dict.copy.description,
    );
  }
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto("/?lang=ru");
    await expect(page.locator("h1")).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    await page.evaluate(async () => {
      for (let y = 0; y < document.body.scrollHeight; y += 700) {
        scrollTo(0, y);
        await new Promise((r) => setTimeout(r, 30));
      }
      scrollTo(0, 0);
    });
    await page.screenshot({
      path: `${dir}/iteration2-full-ru-${width}.png`,
      fullPage: true,
    });
    for (const selector of [
      "#demo",
      ".cv-analytics-section",
      "#plans",
      ".cv-footer",
    ])
      await page
        .locator(selector)
        .screenshot({
          path: `${dir}/iteration2-${selector.replace(/[^a-z]/gi, "")}-${width}.png`,
        });
  }
  expect(errors).toEqual([]);
});
test("demo, tabs, FAQ and mobile navigation are functional without creating data", async ({
  page,
}) => {
  const mutations: string[] = [];
  page.on("request", (r) => {
    if (r.method() !== "GET") mutations.push(r.url());
  });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/?lang=en");
  await page.getByRole("link", { name: en.copy.watch }).click();
  await expect(page).toHaveURL(/#demo$/);
  const demo = page.locator("#demo");
  for (let i = 0; i < 4; i++) {
    await demo
      .getByRole("tab", { name: new RegExp(en.copy.scenarios[i]) })
      .click();
    await expect(
      demo.getByText(en.copy.answers[i], { exact: true }),
    ).toBeVisible();
    if (i === 2) {
      await demo.getByRole("button", { name: en.copy.confirmDemo }).click();
      await expect(demo.getByRole("status")).toHaveText(en.copy.demoSuccess);
    }
    if (i === 3)
      await expect(
        demo.getByText(en.copy.humanAnswer, { exact: false }),
      ).toBeVisible();
  }
  const product = page.locator(".cv-product");
  for (const tab of en.copy.tabs) {
    await product.getByRole("tab", { name: tab, exact: true }).click();
    await expect(
      product.getByRole("tab", { name: tab, exact: true }),
    ).toHaveAttribute("aria-selected", "true");
  }
  await product
    .getByRole("tab", { name: en.copy.tabs[4], exact: true })
    .press("Home");
  await expect(product.getByRole("tab").first()).toBeFocused();
  await page.locator("#faq summary").first().click();
  await expect(
    page.getByText(en.copy.faqs[0].a, { exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => scrollTo(0, 0));
  await page.getByRole("button", { name: en.copy.menu, exact: true }).click();
  await expect(page.locator(".cv-nav")).toBeVisible();
  await page
    .getByRole("button", { name: en.copy.close, exact: true })
    .press("Escape");
  await expect(page.locator(".cv-nav")).not.toBeVisible();
  await page.getByRole("button", { name: en.copy.menu, exact: true }).click();
  await page
    .locator(".cv-nav")
    .getByRole("link", { name: en.copy.nav[3], exact: true })
    .click();
  await expect(page).toHaveURL(/#plans$/);
  await expect(page.locator(".cv-nav")).not.toBeVisible();
  expect(mutations).toEqual([]);
  await page
    .locator(".cv-hero")
    .getByRole("link", { name: en.copy.try })
    .click();
  await expect(page).toHaveURL("/register");
  await expect(
    page.getByLabel("Repeat password", { exact: true }),
  ).toBeVisible();
  expect(await page.locator(".cosmic").count()).toBe(0);
  expect(await page.locator('link[href*="Landing"]').count()).toBe(0);
});
test("free CTA uses real registration, login and organization Trial", async ({
  page,
}) => {
  const email = `cosmic-${Date.now()}@example.com`,
    password = "cosmic-browser-test-1234";
  await page.goto("/?lang=en");
  await page
    .locator(".cv-hero")
    .getByRole("link", { name: en.copy.try })
    .click();
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByLabel("Repeat password", { exact: true }).fill(password);
  await page
    .getByRole("button", { name: "Create account", exact: true })
    .click();
  await expect(page).toHaveURL("/login");
  await page.getByLabel("Email", { exact: true }).fill(email);
  await page.getByLabel("Password", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page).toHaveURL("/app");
  await page
    .getByRole("button", { name: "Create company", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByLabel("Company name")
    .fill("Cosmic synthetic studio");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Save", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Plan and usage", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Trial", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("trialing", { exact: true })).toHaveCount(0);
  await page.reload();
  await page
    .getByRole("button")
    .filter({
      has: page.getByRole("heading", {
        name: "Cosmic synthetic studio",
        exact: true,
      }),
    })
    .click();
  await page
    .getByRole("button", { name: "Plan and usage", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Trial", exact: true }),
  ).toBeVisible();
});
