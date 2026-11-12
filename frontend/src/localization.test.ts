import { describe, it, expect, vi, afterEach } from "vitest";
import en from "./locales/en/translation.json";
import ru from "./locales/ru/translation.json";
import cs from "./locales/cs/translation.json";
import de from "./locales/de/translation.json";
import i18next from "i18next";
import { api, ApiError } from "./api";
function keys(value: object, prefix = ""): string[] {
  return Object.entries(value)
    .flatMap(([k, v]) =>
      typeof v === "object" ? keys(v, prefix + k + ".") : [prefix + k],
    )
    .sort();
}
describe("localization", () => {
  for (const [locale, dict] of Object.entries({ en, ru, cs, de })) {
    it(locale + " has all translations and supports switching", async () => {
      expect(keys(dict)).toEqual(keys(en));
      const i = i18next.createInstance();
      await i.init({
        resources: { [locale]: { translation: dict }, en: { translation: en } },
        lng: locale,
        fallbackLng: "en",
      });
      expect(i.language).toBe(locale);
      expect(i.t("save")).toBe(dict.save);
      expect(
        new Intl.DateTimeFormat(locale).format(new Date("2026-01-01")),
      ).toBeTruthy();
    });
  }
});
afterEach(() => vi.unstubAllGlobals());
it("maps API errors and sends CSRF header", async () => {
  const fetch = vi.fn().mockResolvedValue({
    ok: false,
    status: 403,
    json: async () => ({ error: { code: "forbidden" } }),
  });
  vi.stubGlobal("fetch", fetch);
  await expect(api("/organizations", "POST", {})).rejects.toEqual(
    new ApiError("forbidden", 403),
  );
  expect(fetch.mock.calls[0][1].headers["X-CSRF-Protection"]).toBe("1");
});
it("handles network errors", async () => {
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
  await expect(api("/profile")).rejects.toEqual(new ApiError("network", 0));
});
