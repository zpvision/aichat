// @vitest-environment jsdom
import React from "react";
import { it, expect, vi, afterEach } from "vitest";
import {
  render,
  screen,
  fireEvent,
  cleanup,
  waitFor,
} from "@testing-library/react";
import { Analytics, ratio, change } from "./Analytics";
import { api } from "../api";
import i18n from "../i18n";
import en from "../locales/en/analytics.json";
import ru from "../locales/ru/analytics.json";
import cs from "../locales/cs/analytics.json";
import de from "../locales/de/analytics.json";
vi.mock("../api", async () => ({
  ...(await vi.importActual("../api")),
  api: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  history.replaceState(null, "", "/");
});
const props = {
  organizationId: "org",
  timezone: "UTC",
  sites: [],
  assistants: [],
  onConversation: vi.fn(),
  onLeads: vi.fn(),
  onBilling: vi.fn(),
};
const kpi = {
  dialogs: 0,
  leads: 0,
  eligible: 0,
  converted: 0,
  without_operator: 0,
  handoffs: 0,
  generation_ms: null,
};
const empty = {
  period: {
    start: "2026-01-01T00:00:00Z",
    end: "2026-01-08T00:00:00Z",
    timezone: "UTC",
  },
  kpis: { current: kpi, previous: kpi },
  timeseries: [],
  handling: {
    without_operator: 0,
    with_operator: 0,
    waiting_now: 0,
    human_active_now: 0,
  },
  quality: { no_sources: 0, failed: 0, canceled: 0 },
  executions: [],
  lead_sources: [],
  sites: [],
  sites_total: 0,
  attention: [],
  attention_total: 0,
  operators: [],
  operators_total: 0,
  insights: { top_site: null, top_assistant: null },
};
it("does not invent rates, previous growth or duration on zero/missing data", () => {
  expect(ratio(0, 0)).toBeNull();
  expect(ratio(1, 4)).toBe(25);
  expect(change(3, 0)).toBeNull();
  expect(change(null, 3)).toBeNull();
  expect(change(0, 4)).toBe(-100);
});
it("has structurally complete four-language analytics dictionaries", () => {
  const shape = (x: unknown): unknown =>
    Array.isArray(x)
      ? x.map(shape)
      : x && typeof x === "object"
        ? Object.fromEntries(Object.entries(x).map(([k, v]) => [k, shape(v)]))
        : typeof x;
  for (const c of [ru, cs, de]) expect(shape(c)).toEqual(shape(en));
});
it("renders an honest empty state, applies filters and switches tabs", async () => {
  await i18n.changeLanguage("en");
  vi.mocked(api).mockImplementation(async (path) =>
    path.endsWith("/billing") ? null : (empty as never),
  );
  render(<Analytics {...props} />);
  await screen.findByText(en.empty);
  expect(screen.getAllByText(en.noBase)).toHaveLength(6);
  fireEvent.click(screen.getByRole("tab", { name: en.tabs[2] }));
  expect(screen.getByText(en.teamNote)).toBeTruthy();
  fireEvent.change(screen.getByLabelText(en.from, { exact: true }), {
    target: { value: "2026-01-01" },
  });
  fireEvent.change(screen.getByLabelText(en.to, { exact: true }), {
    target: { value: "2026-01-07" },
  });
  fireEvent.click(screen.getByRole("button", { name: en.apply }));
  await waitFor(() =>
    expect(
      vi.mocked(api).mock.calls.some(([p]) => p.includes("from=2026-01-01")),
    ).toBe(true),
  );
  expect(location.search).toContain("an_from=2026-01-01");
  fireEvent.click(screen.getByRole("button", { name: /Open plan/ }));
  expect(props.onBilling).toHaveBeenCalled();
});
it("shows a localized safe error and supports retry", async () => {
  await i18n.changeLanguage("en");
  vi.mocked(api).mockRejectedValue(new Error("private connection detail"));
  render(<Analytics {...props} />);
  await screen.findByRole("alert");
  expect(screen.getByRole("alert").textContent).toContain(en.error);
  expect(document.body.textContent).not.toContain("private connection");
  fireEvent.click(screen.getByRole("button", { name: en.retry }));
  await waitFor(() =>
    expect(vi.mocked(api).mock.calls.length).toBeGreaterThan(2),
  );
});
