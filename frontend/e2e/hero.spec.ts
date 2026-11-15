import { test, expect } from "@playwright/test";
import { mkdir } from "node:fs/promises";

const dir = "../artifacts/hero-polish";

test("Hero: localized typography, readable cards and reduced motion at four widths", async ({
  page,
}) => {
  test.setTimeout(90000);
  await mkdir(dir, { recursive: true });
  const errors: string[] = [];
  const mutations: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => {
    if (m.type() === "error") errors.push(m.text());
  });
  page.on("request", (r) => {
    if (r.method() !== "GET") mutations.push(r.url());
  });
  await page.emulateMedia({ reducedMotion: "reduce" });
  for (const lang of ["ru", "en", "cs", "de"]) {
    for (const width of [1440, 1920, 390, 360]) {
      await page.setViewportSize({ width, height: width < 500 ? 1500 : 1000 });
      await page.goto("/?lang=" + lang);
      await expect(page.locator("h1")).toBeVisible();
      await page.evaluate(() => document.fonts.ready);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      const hero = page.locator(".cv-hero");
      for (const selector of [
        ".cv-message",
        ".cv-source-note",
        ".cv-demo-caption",
      ]) {
        for (const el of await hero
          .locator(".cv-hero-chat " + selector)
          .all()) {
          await expect(el).toBeVisible();
          expect(
            await el.evaluate((e) => e.scrollWidth <= e.clientWidth + 1),
          ).toBe(true);
        }
      }
      if (width >= 1440) {
        for (const line of await page.locator("h1 > span").all()) {
          expect(
            await line.evaluate(
              (e) =>
                e.clientHeight / parseFloat(getComputedStyle(e).lineHeight),
            ),
          ).toBeLessThan(1.02);
        }
      } else {
        const caption = await hero.locator(".cv-demo-caption").boundingBox();
        const lead = await hero.locator(".cv-floating-lead").boundingBox();
        expect(lead!.y).toBeGreaterThan(caption!.y + caption!.height);
      }
      await expect(hero.locator(".cv-hero-typing")).toBeHidden();
      expect(
        await hero.evaluate((e) => e.getAnimations({ subtree: true }).length),
      ).toBe(0);
      await page.screenshot({ path: `${dir}/after-${lang}-${width}.png` });
    }
  }
  expect(errors).toEqual([]);
  expect(mutations).toEqual([]);
});

test("Hero animation stages messages, typing, reply, lead and handoff without layout shifts", async ({
  page,
}) => {
  await mkdir(dir, { recursive: true });
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: width < 500 ? 1500 : 1000 });
    await page.emulateMedia({ reducedMotion: "no-preference" });
    await page.goto("/?lang=ru");
    await expect(page.locator("h1")).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    const hero = page.locator(".cv-hero");
    const height = await hero.evaluate((e) => e.clientHeight);
    const seek = async (ms: number) => {
      await hero.evaluate((e, time) => {
        for (const a of e.getAnimations({ subtree: true })) {
          a.pause();
          a.currentTime = time;
        }
      }, ms);
      expect(await hero.evaluate((e) => e.clientHeight)).toBe(height);
    };
    const opacity = (selector: string) =>
      hero
        .locator(selector)
        .evaluate((e) => Number(getComputedStyle(e).opacity));
    await seek(0);
    expect(await opacity(".cv-hero-stage")).toBe(0);
    await seek(1500);
    expect(await opacity(".cv-appear-one")).toBe(1);
    expect(await opacity(".cv-appear-two")).toBe(0);
    await seek(2200);
    await expect(hero.locator(".cv-hero-typing")).toBeVisible();
    expect(await opacity(".cv-hero-answer")).toBe(0);
    await page.screenshot({ path: `${dir}/typing-${width}.png` });
    await seek(3250);
    await expect(hero.locator(".cv-hero-typing")).toBeHidden();
    expect(await opacity(".cv-hero-answer")).toBe(1);
    expect(await opacity(".cv-floating-lead")).toBe(0);
    await seek(4200);
    expect(await opacity(".cv-floating-lead")).toBe(1);
    expect(await opacity(".cv-hand-card")).toBe(0);
    await seek(5100);
    expect(await opacity(".cv-hand-card")).toBe(1);
    await page.screenshot({ path: `${dir}/animated-complete-${width}.png` });
  }
});
