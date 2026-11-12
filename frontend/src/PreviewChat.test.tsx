// @vitest-environment jsdom
import React from "react";
import {
  render,
  screen,
  fireEvent,
  waitFor,
  cleanup,
} from "@testing-library/react";
import { beforeEach, afterEach, it, expect, vi } from "vitest";
import { I18nextProvider } from "react-i18next";
import i18n from "./i18n";
import { api, ApiError } from "./api";
import { streamMessage } from "./chat-api";
import { PreviewChat } from "./PreviewChat";
vi.mock("./api", async () => ({
  ...(await vi.importActual("./api")),
  api: vi.fn(),
}));
vi.mock("./chat-api", async () => ({
  ...(await vi.importActual("./chat-api")),
  streamMessage: vi.fn(),
}));
const settings = {
  settings: {
    display_name: "Guide",
    role: "Support",
    company_description: "Company",
    style: "Clear",
    instructions: "Use knowledge",
    no_answer: "",
    language_policy: "auto",
    response_locale: "en",
    offer_operator: true,
  },
  provider: "mock",
  model: "deterministic-demo-v1",
  demo: true,
  max_message_chars: 2000,
};
const conversation = {
  id: "conversation",
  created_at: "2026-01-01T10:00:00Z",
  updated_at: "2026-01-01T10:00:00Z",
  status: "open",
};
const assistant = {
  id: "assistant",
  name: "Guide",
  description: "",
  default_locale: "en" as const,
  language_policy: "visitor",
  status: "draft",
  created_at: "2026-01-01",
};
beforeEach(async () => {
  await i18n.changeLanguage("en");
  vi.mocked(api).mockImplementation(async (path, method) => {
    if (path.endsWith("/ai/settings")) return settings;
    if (method === "POST" && path.endsWith("/conversations"))
      return conversation;
    if (path.endsWith("/conversation"))
      return { conversation, messages: [], executions: [], next_before: null };
    return [];
  });
  vi.mocked(streamMessage).mockReset();
});
afterEach(() => cleanup());
function mount() {
  return render(
    <I18nextProvider i18n={i18n}>
      <PreviewChat
        organizationId="org"
        assistants={[assistant]}
        writable
        initialAssistant="assistant"
      />
    </I18nextProvider>,
  );
}
it("shows demo disclosure and saves assistant instructions", async () => {
  mount();
  await screen.findByText(/Demonstration mode/);
  fireEvent.click(
    screen.getByRole("button", { name: "Assistant instructions" }),
  );
  fireEvent.change(screen.getByLabelText("Additional instructions"), {
    target: { value: "New instructions" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(
      vi
        .mocked(api)
        .mock.calls.some(
          ([path, method, data]) =>
            path.endsWith("/ai/settings") &&
            method === "PATCH" &&
            (data as { instructions: string }).instructions ===
              "New instructions",
        ),
    ).toBe(true),
  );
});
it("sends Enter once with a stable request identifier", async () => {
  vi.mocked(streamMessage).mockImplementation(
    async (_path, input, _signal, onEvent) => {
      onEvent({ type: "delta", data: { text: "Test response" } });
      onEvent({
        type: "complete",
        data: { execution: { status: "completed" } },
      });
    },
  );
  mount();
  await screen.findByText(/Demonstration mode/);
  fireEvent.click(screen.getByRole("button", { name: "New conversation" }));
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "Message" })).toBeTruthy(),
  );
  fireEvent.change(screen.getByRole("textbox", { name: "Message" }), {
    target: { value: "What is the price?" },
  });
  fireEvent.keyDown(screen.getByRole("textbox", { name: "Message" }), {
    key: "Enter",
    shiftKey: false,
  });
  await waitFor(() => expect(streamMessage).toHaveBeenCalledTimes(1));
  expect(vi.mocked(streamMessage).mock.calls[0][1].request_id).toMatch(
    /^[a-f0-9-]{36}$/,
  );
});
it("cancels an in-flight generation", async () => {
  vi.mocked(streamMessage).mockImplementation(
    async (_p, _i, signal) =>
      new Promise((_resolve, reject) =>
        signal.addEventListener("abort", () =>
          reject(new ApiError("ai_canceled", 409)),
        ),
      ),
  );
  mount();
  await screen.findByText(/Demonstration mode/);
  fireEvent.click(screen.getByRole("button", { name: "New conversation" }));
  await screen.findByRole("textbox", { name: "Message" });
  fireEvent.change(screen.getByRole("textbox", { name: "Message" }), {
    target: { value: "Hello" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "Stop generation" }),
  );
  await waitFor(() =>
    expect(vi.mocked(api).mock.calls.some(([p]) => p.endsWith("/cancel"))).toBe(
      true,
    ),
  );
  await screen.findByText("Generation was canceled.");
});
