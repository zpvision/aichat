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
import { api, ApiError } from "./api";
import { LeadCollection } from "./LeadCollection";
import { LeadSettings } from "./LeadSettings";
vi.mock("./api", async () => ({
  ...(await vi.importActual("./api")),
  api: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("requires a separate staff confirmation and sends the version actually displayed", async () => {
  await i18n.changeLanguage("en");
  const posts: unknown[] = [];
  const field = {
    key: "email",
    label: "",
    labels: {},
    type: "email",
    active: true,
    required: false,
  };
  const d = {
    state: "confirmation",
    version: 7,
    conversation_version: 3,
    values: { email: { value: "anna@example.com", valid: true } },
    fields: [field],
    missing: [],
    paused: true,
    enabled: true,
    privacy_url: "https://example.com/privacy",
    error_code: "",
  };
  vi.mocked(api).mockImplementation(async (_, method, body) => {
    if (method === "POST") {
      posts.push(body);
      throw new ApiError("state_conflict", 409);
    }
    return d;
  });
  render(
    <LeadCollection
      organizationId="org"
      conversationId="conversation"
      canWrite
    />,
  );
  await screen.findByText(/anna@example.com/);
  expect(posts).toHaveLength(0);
  fireEvent.click(
    screen.getByRole("button", { name: "Confirm and create lead" }),
  );
  await waitFor(() =>
    expect(posts).toEqual([{ version: 7, conversation_version: 3 }]),
  );
  await screen.findByRole("alert");
  expect(screen.getByText(/anna@example.com/)).toBeTruthy();
});
it("hides manual edits until a staff member takes the conversation", async () => {
  await i18n.changeLanguage("en");
  vi.mocked(api).mockResolvedValue({
    state: "collecting",
    version: 2,
    values: {},
    fields: [],
    missing: [],
    enabled: true,
  });
  render(
    <LeadCollection
      organizationId="org"
      conversationId="conversation"
      canWrite={false}
    />,
  );
  await screen.findByText(
    "Take the conversation before collecting details manually.",
  );
  expect(screen.queryByRole("button", { name: "Edit details" })).toBeNull();
});
it("keeps custom keys editable before save and sends configured field validation", async () => {
  await i18n.changeLanguage("en");
  vi.mocked(api).mockResolvedValue({
    enabled: false,
    revision: 2,
    title: "",
    goal: "",
    require_contact: false,
    success_messages: {},
    fields: [],
  });
  render(
    <LeadSettings
      organizationId="org"
      assistants={[{ id: "a", name: "Guide" } as never]}
    />,
  );
  fireEvent.click(await screen.findByRole("button", { name: "Add field" }));
  const key = screen.getByLabelText("Stable key") as HTMLInputElement;
  fireEvent.change(key, { target: { value: "employee_count" } });
  expect(key.readOnly).toBe(false);
  expect(key.value).toBe("employee_count");
  fireEvent.change(screen.getByLabelText("Field label"), {
    target: { value: "Employees" },
  });
  fireEvent.change(screen.getByLabelText("Type"), {
    target: { value: "number" },
  });
  fireEvent.change(screen.getByLabelText("Minimum"), {
    target: { value: "0" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(
      vi
        .mocked(api)
        .mock.calls.some(
          ([, m, b]) =>
            m === "PUT" &&
            (b as { fields: { key: string; min: number }[] }).fields[0].min ===
              0,
        ),
    ).toBe(true),
  );
});
