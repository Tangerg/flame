import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { useEffect, useRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { contributeForTest } from "@/test/kernel";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import { ChatPanel } from "@/features/shell/workbench/panel/ChatPanel";
import { RunErrorBanner } from "@/features/shell/workbench/panel/RunErrorBanner";
import {
  activateWorkspaceSessionScope,
  openWorkspaceViewInDock,
} from "@/features/workspace/public/navigation";
import {
  replaceComposerDraft,
  useComposerText,
  useSetComposerText,
} from "@/features/composer/public/draft";
import { registerComposerFocusTarget } from "@/features/composer/public/focus";

const INITIAL_DRAFT = "Keep the recovery draft — 保留";

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  useActiveSession: () => null,
  useActiveSessionId: () => "recovery-session",
  useAgentSessions: () => ({ isLoading: false }),
}));
vi.mock("@/features/agent/public/run", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/run")>()),
  useActiveSessionProblem: () => ({
    code: "provider_rejected",
    recovery: "stop",
    message: "The configured model refused this input.",
  }),
}));
vi.mock("@/features/shell/workbench/panel/RunStatusPill", () => ({ RunStatusPill: () => null }));
vi.mock("@/features/shell/workbench/panel/RunAnnouncer", () => ({ RunAnnouncer: () => null }));
vi.mock("@/features/shell/workbench/panel/HeaderDiffStat", () => ({ HeaderDiffStat: () => null }));
vi.mock("@/features/shell/workbench/panel/ChatStream", () => ({
  ChatStream: () => (
    <>
      <RunErrorBanner />
      <DraftInput />
    </>
  ),
}));

function DraftInput() {
  const input = useRef<HTMLTextAreaElement>(null);
  const value = useComposerText();
  const setValue = useSetComposerText();
  useEffect(() => registerComposerFocusTarget(testClient.composer.access, input.current), []);
  return (
    <textarea
      ref={input}
      aria-label="Composer draft"
      value={value}
      onChange={(event) => setValue(event.target.value)}
    />
  );
}

beforeEach(async () => {
  testClient.agent.access.selectAgentSession("recovery-session");
  activateWorkspaceSessionScope(testClient.workspace.access, "recovery-session");
  replaceComposerDraft(testClient.composer.access, { text: INITIAL_DRAFT });
  await contributeForTest((ctx) => {
    for (const view of [
      { id: "file", title: "Files" },
      { id: "diagnostics", title: "Diagnostics", material: "diagnostics" as const },
    ]) {
      ctx.contribute(WORKSPACE_VIEW, {
        ...view,
        dock: "workspace",
        component: () => <input aria-label="Material note" defaultValue="Retained material note" />,
      });
    }
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("Run recovery material navigation", () => {
  it.each([
    { width: 700, action: "Inspect context", title: "Files" },
    { width: 700, action: "Diagnostics", title: "Diagnostics" },
    { width: 1200, action: "Inspect context", title: "Files" },
    { width: 1200, action: "Diagnostics", title: "Diagnostics" },
  ])(
    "opens $action at $width px and returns to the same source and draft",
    ({ width, action, title }) => {
      vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(width);
      openWorkspaceViewInDock(testClient.workspace.access, "file");
      testClient.render(<ChatPanel onSend={() => true} />);
      const draft = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
      const source = screen.getByRole("button", { name: action });
      act(() => source.focus());
      fireEvent.click(source);

      const material = screen.getByRole(width === 700 ? "region" : "complementary", {
        name: title,
      });
      expect(document.activeElement).toBe(material);
      expect(testClient.workspace.dock.store.getState().inspection?.origin).toBe(source);
      const note = screen.getByRole<HTMLInputElement>("textbox", { name: "Material note" });
      fireEvent.change(note, { target: { value: "Read before returning" } });
      act(() => note.focus());
      fireEvent.keyDown(note, { key: "Escape" });

      expect(document.activeElement).toBe(source);
      expect(screen.getByRole("textbox", { name: "Composer draft" })).toBe(draft);
      expect(draft.value).toBe(INITIAL_DRAFT);
      fireEvent.change(draft, { target: { value: INITIAL_DRAFT + " Continued." } });
      act(() => source.focus());
      fireEvent.click(source);
      expect(screen.getByRole<HTMLInputElement>("textbox", { name: "Material note" })).toBe(note);
      expect(note.value).toBe("Read before returning");
      const back = screen.getByRole("button", { name: "Back to conversation" });
      act(() => back.focus());
      fireEvent.click(back);
      expect(document.activeElement).toBe(source);
      expect(draft.value).toBe(INITIAL_DRAFT + " Continued.");
    },
  );
});
