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
import { I18nextProvider } from "react-i18next";
import i18n from "./i18n";
import { api, ApiError } from "./api";
import { Inbox } from "./Inbox";
vi.mock("./api", async () => ({
  ...(await vi.importActual("./api")),
  api: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
it("reuses the staff reply idempotency key after a lost response", async () => {
  await i18n.changeLanguage("en");
  const c = {
    id: "conversation",
    site: "Website",
    assistant: "Guide",
    control_mode: "human_active",
    state_version: 2,
    assigned_user_id: "me",
    assignee: "Colleague",
    unread: 0,
    last_content: "Question",
    created_at: "2026-01-01T10:00:00Z",
    last_message_at: null,
    generating: false,
  };
  const page = {
    conversation: c,
    messages: [],
    events: [],
    cursor: 1,
    next_before: 0,
    more: false,
  };
  const sent: Record<string, unknown>[] = [];
  vi.mocked(api).mockImplementation(async (path, method, body) => {
    if (path.endsWith("/members")) return [];
    if (path.includes("/conversations?"))
      return { items: [c], next_cursor: "" };
    if (method === "POST" && path.endsWith("/messages")) {
      sent.push(body as Record<string, unknown>);
      if (sent.length === 1) throw new ApiError("network", 0);
      return {
        id: "message",
        sequence: 1,
        change_seq: 2,
        role: "operator",
        content: "Human reply",
        status: "completed",
        created_at: c.created_at,
        author_user_id: "me",
      };
    }
    return page;
  });
  render(
    <I18nextProvider i18n={i18n}>
      <Inbox
        organizationId="org"
        userId="me"
        role="operator"
        sites={[]}
        assistants={[]}
      />
    </I18nextProvider>,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: /Visitor #conver/ }),
  );
  const input = await screen.findByRole("textbox", {
    name: "Staff reply",
  });
  fireEvent.change(input, { target: { value: "Human reply" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await screen.findByRole("alert");
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(sent).toHaveLength(2));
  expect(sent[0].request_id).toEqual(sent[1].request_id);
  await waitFor(() => expect((input as HTMLTextAreaElement).value).toBe(""));
});
