import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createFlameClient, type FlameClient } from "@flame/runtime-client";
import { CodeHighlightProvider, FontScaleProvider, TooltipProvider } from "@flame/ui";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { agentTextInput } from "@/features/agent/public/input";
import { executeCommand } from "@/features/command/public/commands";
import { useShortcutOverrides } from "@/features/command/public/shortcutOverrides";
import { showWorkspaceDock } from "@/features/workspace/public/navigation";
import { HighlightedCodeSource } from "@/lib/highlight/HighlightedCodeSource";
import * as clipboard from "@/lib/clipboard";
import { activeLocale, setLocale } from "@/lib/i18n/index";
import { queryClient } from "@/lib/queryClient";
import { resetKernelForTest } from "@/test/kernel";
import { createVisualClient, type VisualClient } from "./client";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import {
  RUNTIME_AGENT_SESSION_SNAPSHOTS,
  VISUAL_ROOT_RUN_ID,
  VISUAL_SESSION_ID,
  type VisualAgentState,
} from "./agentSessionSnapshots";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { installVisualAgentFixture } from "./installVisualAgentFixture";
import { VisualAgentStateFixture } from "./VisualAgentStateFixture";

let visual: VisualClient | undefined;
let client: FlameClient | undefined;
let previousLocale: string;

beforeEach(async () => {
  previousLocale = activeLocale();
  setLocale("en");
  visual = undefined;
  client = undefined;
  useShortcutOverrides.setState({ overrides: {}, recording: false });
  await import("@/features/chat/message/ui/markdown/MarkdownRenderer");
});

afterEach(async () => {
  cleanup();
  await resetKernelForTest();
  await visual?.dispose();
  await client?.close();
  queryClient.clear();
  setLocale(previousLocale);
  vi.restoreAllMocks();
  delete document.documentElement.dataset.visualSentInput;
});

async function mountAgent(state: VisualAgentState) {
  const snapshot = structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS[state]);
  const transport = createVisualRuntimeTransport(snapshot, VISUAL_CAPABILITIES, async () => {
    throw new Error("The Agent fixture has no workspace diff read");
  });
  const requests = vi.spyOn(transport, "send");
  const runtime = createFlameClient(transport, { capabilities: () => VISUAL_CAPABILITIES });
  client = runtime;
  const owned = createVisualClient(() => runtime);
  visual = owned;
  const view = await installVisualAgentFixture(
    owned,
    () => runtime,
    state,
    undefined,
    snapshot,
    VISUAL_CAPABILITIES,
  );
  const original = owned.agent.dependencies.view.getSession(VISUAL_SESSION_ID);
  if (!original?.send)
    throw new Error("The recorded Agent send action must be installed before rendering");
  const sendInput = vi.fn(original.send);
  owned.agent.projection.store.getState().setSend(VISUAL_SESSION_ID, sendInput);
  const createSession = vi.spyOn(owned.agent.dependencies.runtime, "createSession");
  owned.workspace.layout.setState({ sidebarCollapsed: false });
  render(
    <owned.Wrapper>
      <QueryClientProvider client={queryClient}>
        <FontScaleProvider fontSize={14}>
          <CodeHighlightProvider component={HighlightedCodeSource}>
            <TooltipProvider>
              <VisualAgentStateFixture state={state} view={view} />
            </TooltipProvider>
          </CodeHighlightProvider>
        </FontScaleProvider>
      </QueryClientProvider>
    </owned.Wrapper>,
  );
  return { visual: owned, sendInput, createSession, requests };
}

function pressEscape(target: Element) {
  fireEvent(
    target,
    new KeyboardEvent("keydown", {
      key: "Escape",
      code: "Escape",
      bubbles: true,
      cancelable: true,
    }),
  );
}

function pressModified(target: Element, key: string, code: string) {
  const mac = /Mac|iPod|iPhone|iPad/.test(navigator.platform);
  fireEvent(
    target,
    new KeyboardEvent("keydown", {
      key,
      code,
      metaKey: mac,
      ctrlKey: !mac,
      bubbles: true,
      cancelable: true,
    }),
  );
}

function pressShiftF10(target: Element) {
  fireEvent(
    target,
    new KeyboardEvent("keydown", {
      key: "F10",
      code: "F10",
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    }),
  );
}

async function messageCopyAction(author: string) {
  const heading = await screen.findByRole("heading", { name: author, level: 2 });
  return within(heading.parentElement!).findByRole<HTMLButtonElement>("button", {
    name: "Copy message",
  });
}

async function openCopyMenu(copy: HTMLButtonElement) {
  act(() => {
    fireEvent.keyDown(copy, { key: "Enter", code: "Enter" });
    // Happy DOM does not synthesize a native button's Enter click.
    fireEvent.click(copy, { detail: 0 });
  });
  const menu = await screen.findByRole("menu", { name: "Copy message" });
  await waitFor(() => expect(menu.contains(document.activeElement)).toBe(true));
  const markdown = within(menu).getByRole("menuitem", { name: /^Copy markdown\b/ });
  const plain = within(menu).getByRole("menuitem", { name: /^Copy plain text\b/ });
  act(() => markdown.focus());
  return { menu, markdown, plain };
}

it.each([
  {
    author: "You",
    present: ["Edit in composer", "Edit & rerun from here", "Fork up to here"],
    absent: ["Regenerate response"],
  },
  {
    author: "Assistant",
    present: ["Regenerate response"],
    absent: ["Edit in composer", "Fork up to here"],
  },
])(
  "opens the $author message menu from its actual Copy action and returns to authoring",
  async ({ author, present, absent }) => {
    const copied = vi.spyOn(clipboard, "writeToClipboard").mockResolvedValue(true);
    const { sendInput, createSession, requests } = await mountAgent("idle");
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    const value = "Keep this message menu draft — 保留";
    fireEvent.change(draft, { target: { value } });
    const copy = await messageCopyAction(author);
    act(() => {
      draft.focus();
      draft.setSelectionRange(7, 7);
      copy.focus();
    });

    pressShiftF10(copy);

    const menu = await screen.findByRole("menu", { name: author });
    await waitFor(() => expect(menu.contains(document.activeElement)).toBe(true));
    expect(within(menu).getByRole("menuitem", { name: "Copy plain text" })).toBeTruthy();
    for (const name of present) expect(within(menu).getByRole("menuitem", { name })).toBeTruthy();
    for (const name of absent) expect(within(menu).queryByRole("menuitem", { name })).toBeNull();
    pressEscape(document.activeElement!);
    await waitFor(() => expect(screen.queryByRole("menu", { name: author })).toBeNull());
    expect(await messageCopyAction(author)).toBe(copy);
    await waitFor(() => expect(document.activeElement).toBe(copy));
    expect(draft.value).toBe(value);
    expect([draft.selectionStart, draft.selectionEnd]).toEqual([7, 7]);

    const child = await openCopyMenu(copy);
    expect(screen.queryByRole("menu", { name: author })).toBeNull();
    fireEvent.keyDown(child.markdown, { key: "ArrowDown", code: "ArrowDown" });
    await waitFor(() => expect(document.activeElement).toBe(child.plain));
    pressEscape(child.plain);
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(copy));

    await act(() => executeCommand("composer.focus"));
    expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
    expect(document.activeElement).toBe(draft);
    expect(draft.value).toBe(value);
    expect([draft.selectionStart, draft.selectionEnd]).toEqual([7, 7]);
    expect(copied).not.toHaveBeenCalled();
    expect(sendInput).not.toHaveBeenCalled();
    expect(createSession).not.toHaveBeenCalled();
    expect(
      requests.mock.calls.filter(([request]) => /^(sessions|runs)\./.test(request.method)),
    ).toEqual([]);
  },
);

it.each(["keyboard", "pointer"] as const)(
  "keeps the portaled Copy menu as the owner of a %s context request",
  async (entry) => {
    const copied = vi.spyOn(clipboard, "writeToClipboard").mockResolvedValue(true);
    const { sendInput, createSession, requests } = await mountAgent("idle");
    const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
    const value = "Keep the nested Copy task — 保留";
    fireEvent.change(draft, { target: { value } });
    const copy = await messageCopyAction("You");
    act(() => {
      draft.focus();
      draft.setSelectionRange(5, 5);
      copy.focus();
    });
    const { menu, markdown, plain } = await openCopyMenu(copy);
    fireEvent.keyDown(markdown, { key: "ArrowDown", code: "ArrowDown" });
    await waitFor(() => expect(document.activeElement).toBe(plain));

    act(() => {
      if (entry === "keyboard") pressShiftF10(plain);
      else fireEvent.contextMenu(plain, { button: 2 });
    });

    expect(screen.queryByRole("menu", { name: "You" })).toBeNull();
    expect(screen.getByRole("menu", { name: "Copy message" })).toBe(menu);
    expect(document.activeElement).toBe(plain);
    expect(within(menu).getByRole("menuitem", { name: /^Copy plain text\b/ })).toBe(plain);
    fireEvent.keyDown(plain, { key: "ArrowUp", code: "ArrowUp" });
    await waitFor(() => expect(document.activeElement).toBe(markdown));
    pressEscape(markdown);
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(copy));
    await act(() => executeCommand("composer.focus"));
    expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
    expect(document.activeElement).toBe(draft);
    expect(draft.value).toBe(value);
    expect([draft.selectionStart, draft.selectionEnd]).toEqual([5, 5]);
    expect(copied).not.toHaveBeenCalled();
    expect(sendInput).not.toHaveBeenCalled();
    expect(createSession).not.toHaveBeenCalled();
    expect(
      requests.mock.calls.filter(([request]) => /^(sessions|runs)\./.test(request.method)),
    ).toEqual([]);
  },
);

it("returns from the Agent sidebar through the installed Composer command and its key", async () => {
  const { sendInput } = await mountAgent("tool-remote");
  const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
  const value = "  Keep this Agent draft — 保留  ";
  fireEvent.change(draft, { target: { value } });
  const sidebar = screen.getByRole("button", { name: "Collapse the agent fixture sidebar" });
  act(() => {
    draft.focus();
    draft.setSelectionRange(7, 7);
    sidebar.focus();
  });

  await act(() => executeCommand("composer.focus"));

  expect(document.activeElement).toBe(draft);
  expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
  expect(draft.value).toBe(value);
  expect([draft.selectionStart, draft.selectionEnd]).toEqual([7, 7]);
  act(() => sidebar.focus());
  pressModified(sidebar, "l", "KeyL");
  expect(document.activeElement).toBe(draft);
  expect(draft.value).toBe(value);
  expect([draft.selectionStart, draft.selectionEnd]).toEqual([7, 7]);
  expect(sendInput).not.toHaveBeenCalled();
});

it("closes the Agent's full-width catalogue through the global Escape binding", async () => {
  const { visual, sendInput } = await mountAgent("tool-remote");
  const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
  fireEvent.change(draft, { target: { value: "Keep the catalogue source — 保留" } });
  act(() => showWorkspaceDock(visual.workspace.access, "main"));
  const catalogue = await screen.findByRole("region", { name: "Open a panel" });
  act(() => catalogue.focus());

  pressEscape(catalogue);

  expect(visual.navigation.get().view).toBeNull();
  expect(screen.queryByRole("region", { name: "Open a panel" })).toBeNull();
  expect(screen.getByRole("textbox", { name: "Message composer" })).toBe(draft);
  expect(draft.value).toBe("Keep the catalogue source — 保留");
  expect(sendInput).not.toHaveBeenCalled();
});

it("opens the existing shortcut Settings command and returns to the same Agent draft", async () => {
  const { visual } = await mountAgent("tool-remote");
  const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
  fireEvent.change(draft, { target: { value: "Keep the shortcut source — 保留" } });
  const sidebar = screen.getByRole("button", { name: "Collapse the agent fixture sidebar" });
  act(() => {
    draft.focus();
    draft.setSelectionRange(5, 5);
    sidebar.focus();
  });
  expect(
    await screen.findByRole("tooltip", { name: "Collapse the agent fixture sidebar" }),
  ).toBeTruthy();

  pressModified(sidebar, "/", "Slash");

  expect(visual.navigation.get().view).toBe("settings");
  expect(visual.navigation.get().settings).toBe("shortcuts");
  expect(await screen.findByRole("searchbox", { name: "Filter shortcuts" })).toBeTruthy();
  const tab = screen.getByRole("tab", { name: "Keyboard shortcuts" });
  act(() => tab.focus());
  pressEscape(tab);
  expect(visual.navigation.get().view).toBe("settings");
  expect(draft.isConnected).toBe(true);
  expect(draft.value).toBe("Keep the shortcut source — 保留");
  expect(tab.isConnected).toBe(true);
  expect(document.activeElement).toBe(tab);
  pressEscape(tab);
  expect(visual.navigation.get().view).toBeNull();
  await act(() => executeCommand("composer.focus"));
  expect(document.activeElement).toBe(draft);
  expect(draft.value).toBe("Keep the shortcut source — 保留");
  expect([draft.selectionStart, draft.selectionEnd]).toEqual([5, 5]);
});

it.each([
  { state: "empty", selected: "", status: undefined, interrupt: null },
  { state: "runtime-down", selected: "", status: undefined, interrupt: null },
  { state: "waiting", selected: VISUAL_SESSION_ID, status: "waiting", interrupt: "approval" },
  { state: "question", selected: VISUAL_SESSION_ID, status: "waiting", interrupt: "question" },
  { state: "canceled", selected: VISUAL_SESSION_ID, status: "finished", interrupt: "closed" },
  { state: "running", selected: VISUAL_SESSION_ID, status: "running", interrupt: null },
  { state: "recovery", selected: VISUAL_SESSION_ID, status: "finished", interrupt: null },
] as const)(
  "keeps the captured $state context without Session or Run dispatch",
  async (expected) => {
    const { visual, sendInput, createSession, requests } = await mountAgent(expected.state);
    expect(visual.agent.access.getActiveSessionId()).toBe(expected.selected);
    expect(
      visual.agent.dependencies.view.getSession(VISUAL_SESSION_ID)?.view.runsById[
        VISUAL_ROOT_RUN_ID
      ]?.status,
    ).toBe(expected.status);
    expect(Boolean(screen.queryByRole("textbox", { name: "Message composer" }))).toBe(
      expected.interrupt !== "question",
    );
    expect(Boolean(screen.queryByRole("button", { name: "Approval options" }))).toBe(
      expected.interrupt === "approval",
    );
    expect(Boolean(screen.queryByRole("button", { name: "Skip" }))).toBe(
      expected.interrupt === "question",
    );
    expect(Boolean(screen.queryByText("Closed without an answer"))).toBe(
      expected.interrupt === "closed",
    );
    expect(createSession).not.toHaveBeenCalled();
    expect(sendInput).not.toHaveBeenCalled();
    expect(
      requests.mock.calls.filter(([request]) => /^(sessions|runs)\./.test(request.method)),
    ).toEqual([]);
  },
);

it("retries the recovery scene's original user input once", async () => {
  const { sendInput } = await mountAgent("recovery");
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
  await waitFor(() => expect(sendInput).toHaveBeenCalledOnce());
  expect(sendInput.mock.calls[0]?.[0]).toEqual(
    agentTextInput(
      "Review the Runtime boundary and keep application-owned atomicity out of the Agent Framework.",
    ),
  );
  expect(screen.queryAllByRole("alert")).toHaveLength(0);
});
