import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { GoalComposerModeOwner } from "@/features/goal/application/goalComposerMode";
import { GoalModeIndicator } from "@/features/goal/ui/GoalModeIndicator";
const model = vi.hoisted(() => ({
  sessionId: "session-a",
  goal: null as { status: "active" | "paused" | "blocked" | "completing" } | null,
}));

vi.mock("@/features/agent/public/session", () => ({
  useActiveSessionId: () => model.sessionId,
}));

vi.mock("@/features/goal/application/goalReadModel", () => ({
  useGoalMaterial: () => ({ value: { available: true, goal: model.goal } }),
}));

let owner: GoalComposerModeOwner;

beforeEach(() => {
  model.sessionId = "session-a";
  model.goal = null;
  owner = GoalComposerModeOwner.install();
});

afterEach(async () => {
  cleanup();
  owner.dispose();
  await drainBrowserTasks();
});

describe("GoalModeIndicator", () => {
  it("renders no permanent Goal launcher while composer mode is inactive", () => {
    testClient.render(<GoalModeIndicator />);

    expect(screen.queryByText("Goal")).toBeNull();
    expect(screen.queryAllByRole("spinbutton")).toHaveLength(0);
  });

  it("renders the compact Codex-style mode indicator and clears it in place", () => {
    owner.activate("session-a");
    testClient.render(<GoalModeIndicator />);

    const indicator = screen.getByRole("button", { name: "Exit Goal mode" });
    expect(indicator.getAttribute("aria-pressed")).toBeNull();
    expect(screen.getByText("Goal")).toBeTruthy();
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(screen.queryAllByRole("spinbutton")).toHaveLength(0);

    fireEvent.click(indicator);
    expect(screen.queryByText("Goal")).toBeNull();
  });

  it.each(["", "Keep this objective draft."])(
    "returns an accepted exit to the same editable draft (%j)",
    (draft) => {
      owner.activate("session-a");
      testClient.render(
        <>
          <textarea
            aria-label="Message composer"
            defaultValue={draft}
            ref={testClient.composer.access.registerFocusTarget}
          />
          <GoalModeIndicator />
        </>,
      );
      const input = screen.getByRole("textbox", {
        name: "Message composer",
      }) as HTMLTextAreaElement;
      input.focus();
      input.setSelectionRange(2, 8);
      const selection = [input.selectionStart, input.selectionEnd];
      const exit = screen.getByRole("button", { name: "Exit Goal mode" });
      exit.focus();

      fireEvent.click(exit);

      expect(screen.queryByRole("button", { name: "Exit Goal mode" })).toBeNull();
      expect(document.activeElement).toBe(input);
      expect(input.value).toBe(draft);
      expect([input.selectionStart, input.selectionEnd]).toEqual(selection);
      expect(owner.snapshot().phase).toBe("inactive");
    },
  );

  it("keeps later source focus when the mode exits", () => {
    owner.activate("session-a");
    testClient.render(
      <>
        <textarea
          aria-label="Message composer"
          defaultValue="Keep this draft."
          ref={testClient.composer.access.registerFocusTarget}
        />
        <GoalModeIndicator />
        <button>Later source action</button>
      </>,
    );
    const later = screen.getByRole("button", { name: "Later source action" });
    const release = owner.subscribe(() => later.focus());
    const exit = screen.getByRole("button", { name: "Exit Goal mode" });
    exit.focus();

    fireEvent.click(exit);

    expect(screen.queryByRole("button", { name: "Exit Goal mode" })).toBeNull();
    expect(document.activeElement).toBe(later);
    release();
  });

  it("keeps replacement confirmation separate from the composer draft", () => {
    owner.activate("session-a");
    owner.requestReplacement("session-a", "old objective", vi.fn());
    testClient.render(<GoalModeIndicator />);

    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(screen.getByText(/old objective/)).toBeTruthy();
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(screen.queryAllByRole("spinbutton")).toHaveLength(0);

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(owner.snapshot().phase).toBe("armed");
  });

  it("keeps the starting owner until its command settles when standing material arrives first", () => {
    owner.activate("session-a");
    owner.begin("session-a");
    const { rerender } = testClient.render(<GoalModeIndicator />);

    model.goal = { status: "active" };
    rerender(<GoalModeIndicator />);

    expect(owner.snapshot()).toMatchObject({ sessionId: "session-a", phase: "starting" });
  });
});
