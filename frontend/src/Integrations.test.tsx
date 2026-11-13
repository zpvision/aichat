// @vitest-environment jsdom
import React from "react";
import { it, expect, vi, afterEach } from "vitest";
import {
  render,
  screen,
  fireEvent,
  waitFor,
  cleanup,
} from "@testing-library/react";
import i18n from "./i18n";
import { api } from "./api";
import { Integrations } from "./Integrations";
vi.mock("./api", async () => ({
  ...(await vi.importActual("./api")),
  api: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("shows a newly created secret once without persisting it to browser storage", async () => {
  await i18n.changeLanguage("en");
  vi.mocked(api).mockImplementation(async (_, method) =>
    method === "POST" ? { id: "key", token: "one-time-key" } : [],
  );
  const storage = vi.spyOn(Storage.prototype, "setItem");
  const view = render(<Integrations organizationId="org" />);
  fireEvent.change(screen.getByLabelText("Key name"), {
    target: { value: "CRM" },
  });
  fireEvent.click(screen.getByLabelText("Allow creating leads"));
  fireEvent.click(screen.getByRole("button", { name: "Create API key" }));
  expect(
    ((await screen.findByLabelText("New secret")) as HTMLTextAreaElement).value,
  ).toBe("one-time-key");
  expect(storage).not.toHaveBeenCalled();
  await waitFor(() =>
    expect(
      vi
        .mocked(api)
        .mock.calls.some(
          ([, method, body]) =>
            method === "POST" &&
            (body as { scopes: string[] }).scopes.includes("leads:write"),
        ),
    ).toBe(true),
  );
  fireEvent.click(screen.getByRole("button", { name: "Saved — hide secret" }));
  expect(screen.queryByLabelText("New secret")).toBeNull();
  view.unmount();
  render(<Integrations organizationId="org" />);
  expect(screen.queryByLabelText("New secret")).toBeNull();
  storage.mockRestore();
});
