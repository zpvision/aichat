// Original vector artwork -> social PNG. No remote assets or services.
import { chromium } from "@playwright/test";
import { readFile, writeFile } from "node:fs/promises";
const source = await readFile(
  new URL("../public/cosmic-social.svg", import.meta.url),
  "utf8",
);
const browser = await chromium.launch();
try {
  const page = await browser.newPage();
  const png = await page.evaluate(async (svg) => {
    const image = new Image();
    image.src =
      "data:image/svg+xml;base64," + btoa(unescape(encodeURIComponent(svg)));
    await image.decode();
    const canvas = document.createElement("canvas");
    canvas.width = 1200;
    canvas.height = 630;
    canvas.getContext("2d").drawImage(image, 0, 0);
    return canvas.toDataURL("image/png").split(",")[1];
  }, source);
  await writeFile(
    new URL("../public/cosmic-social.png", import.meta.url),
    Buffer.from(png, "base64"),
  );
} finally {
  await browser.close();
}
