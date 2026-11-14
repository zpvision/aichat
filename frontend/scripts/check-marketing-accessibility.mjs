// Optional local audit tool; axe is not bundled with the product.
// AXE_SOURCE_FILE must point to a trusted local axe-core 4.11.x axe.min.js.
import { chromium } from "@playwright/test";
import { mkdir, writeFile } from "node:fs/promises";
if (!process.env.AXE_SOURCE_FILE)
  throw new Error("Set AXE_SOURCE_FILE to a trusted local axe-core script");
const browser = await chromium.launch(),
  checks = [];
try {
  const page = await browser.newPage();
  await page.emulateMedia({ reducedMotion: "reduce" });
  for (const [language, width] of [
    ["en", 1440],
    ["ru", 390],
    ["de", 360],
    ["cs", 768],
  ]) {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(
      `${process.env.MARKETING_TEST_URL || "http://127.0.0.1:5174"}/?lang=${language}`,
    );
    await page.locator("h1").waitFor();
    await page.evaluate(() => document.fonts.ready);
    await page.addScriptTag({ path: process.env.AXE_SOURCE_FILE });
    for (let tab = 0; tab < 5; tab++) {
      await page.locator(".cv-product-tabs button").nth(tab).click();
      const report = await page.evaluate(() =>
        axe.run(document.querySelector(".cosmic"), {
          runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa"] },
        }),
      );
      checks.push({
        language,
        width,
        tab,
        violations: report.violations.map((v) => ({
          id: v.id,
          nodes: v.nodes.map((n) => ({
            target: n.target,
            reason: n.failureSummary,
          })),
        })),
      });
    }
  }
  await mkdir("../artifacts/stage9a2", { recursive: true });
  await writeFile(
    "../artifacts/stage9a2/accessibility.json",
    JSON.stringify({ engine: "axe-core 4.11.0", checks }, null, 2),
  );
  const violations = checks.filter((c) => c.violations.length);
  console.log(
    JSON.stringify(
      violations.length ? violations : { checks: checks.length, violations: 0 },
      null,
      2,
    ),
  );
  if (violations.length) process.exitCode = 1;
} finally {
  await browser.close();
}
