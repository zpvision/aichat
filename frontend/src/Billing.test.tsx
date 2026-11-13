// @vitest-environment jsdom
import React from "react";
import { it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import i18n from "./i18n";
import { api } from "./api";
import { Billing } from "./Billing";
vi.mock("./api", async () => ({
  ...(await vi.importActual("./api")),
  api: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
const snapshot = {
  subscription: {
    status: "expired",
    region: "eu",
    trial_end: "2026-01-15T00:00:00Z",
  },
  plan: { name: "Trial", version: 1 },
  period: {
    id: "period",
    starts_at: "2026-01-01T00:00:00Z",
    ends_at: "2026-01-15T00:00:00Z",
    limits: {
      ai_responses: 0,
      demo_responses: 500,
      assistants: null,
      sites: 3,
      members: 5,
      knowledge_sources: 30,
      knowledge_bytes: 52428800,
    },
    ai_used: 0,
    ai_reserved: 0,
    demo_used: 2,
    demo_reserved: 0,
  },
  resources: { members: 1 },
  uncounted_files: 0,
  periods: [],
  technical: { executions: 2, input_tokens: null, output_tokens: null },
};
it("shows expiration, unlimited and zero separately without a fake payment action", async () => {
  await i18n.changeLanguage("en");
  vi.mocked(api).mockResolvedValue(snapshot);
  render(<Billing organizationId="org" />);
  await screen.findByText("Trial");
  expect(screen.getByText(/No plan limit/)).toBeTruthy();
  expect(screen.getByText("Unavailable on this plan")).toBeTruthy();
  expect(screen.getByText(/staff replies remain available/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: /pay|buy/i })).toBeNull();
});
