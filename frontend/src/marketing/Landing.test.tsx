// @vitest-environment jsdom
import React from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  render,
  screen,
  fireEvent,
  cleanup,
  act,
} from "@testing-library/react";
import i18n from "../i18n";
import Landing from "./Landing";
import en from "../locales/en/marketing.json";
import ru from "../locales/ru/marketing.json";
import cs from "../locales/cs/marketing.json";
import de from "../locales/de/marketing.json";
beforeEach(() => {
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.stubGlobal("matchMedia", () => ({ matches: true }));
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue({
        ok: true,
        json: async () => ({ brand_name: "Site Assistant" }),
      }),
  );
  history.replaceState(null, "", "/");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
it("keeps every marketing dictionary structurally complete, with stable technical plan names", () => {
  function shape(x: unknown): unknown {
    return Array.isArray(x)
      ? x.map(shape)
      : x !== null && typeof x === "object"
        ? Object.fromEntries(Object.entries(x).map(([k, v]) => [k, shape(v)]))
        : typeof x;
  }
  for (const d of [ru, cs, de]) expect(shape(d)).toEqual(shape(en));
  for (const d of [en, ru, cs, de]) {
    expect(d.copy.plans.map((p) => p.name)).toEqual([
      "Trial",
      "Start",
      "Business",
    ]);
    expect(d.copy.faqs).toHaveLength(9);
  }
});
it("renders the requested slogan in each language and links real registration and login routes", async () => {
  for (const [locale, d] of Object.entries({ en, ru, cs, de })) {
    await act(async () => {
      await i18n.changeLanguage(locale);
      render(<Landing />);
    });
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(
      d.copy.hero.join(""),
    );
    expect(
      screen
        .getAllByRole("link", { name: d.copy.start })[0]
        .getAttribute("href"),
    ).toBe("/register");
    expect(
      screen
        .getAllByRole("link", { name: d.copy.login })[0]
        .getAttribute("href"),
    ).toBe("/login");
    expect(document.documentElement.lang).toBe(locale);
    cleanup();
  }
});
it("plays a synthetic request, confirms locally and switches product tabs without a mutation request", async () => {
  vi.useFakeTimers();
  await i18n.changeLanguage("en");
  await act(async () => {
    render(<Landing />);
  });
  fireEvent.click(screen.getByRole("tab", { name: /Leave a request/ }));
  await act(async () => {
    vi.runAllTimers();
  });
  fireEvent.click(screen.getByRole("button", { name: en.copy.confirmDemo }));
  expect(screen.getByRole("status").textContent).toBe(en.copy.demoSuccess);
  fireEvent.click(screen.getByRole("tab", { name: en.copy.tabs[4] }));
  expect(
    screen
      .getByRole("tab", { name: en.copy.tabs[4] })
      .getAttribute("aria-selected"),
  ).toBe("true");
  expect(
    vi
      .mocked(fetch)
      .mock.calls.every(
        ([, options]) =>
          !options || !options.method || options.method === "GET",
      ),
  ).toBe(true);
});
