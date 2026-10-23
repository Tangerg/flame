import { createRef, type ComponentProps, type ReactNode } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { type RuntimeServiceSnapshot } from "@/features/connection/public/serviceStatus";
import { FloatingComposer } from "@/features/shell/workbench/panel/FloatingComposer";
const runtime = vi.hoisted(() => ({
  snapshot: {
    phase: "reconnecting",
    observation: null,
    failure: null,
  } as RuntimeServiceSnapshot,
}));

const navigation = vi.hoisted(() => ({
  openSettings: vi.fn(),
}));

vi.mock("@/features/connection/public/serviceStatus", () => ({
  useRuntimeServiceStatus: () => runtime.snapshot,
}));

vi.mock("motion/react", () => ({
  AnimatePresence: ({ children }: { children: ReactNode }) => children,
  motion: {
    div: ({
      children,
      initial: _initial,
      animate: _animate,
      exit: _exit,
      transition: _transition,
      ...props
    }: ComponentProps<"div"> & {
      initial?: unknown;
      animate?: unknown;
      exit?: unknown;
      transition?: unknown;
    }) => <div {...props}>{children}</div>,
  },
}));

vi.mock("@/features/shell/workbench/panel/JumpToBottomButton", () => ({
  JumpToBottomButton: () => <button>Jump to latest message</button>,
}));

vi.mock("@/features/workspace/public/navigation", () => ({
  openWorkspaceSettingsPane: navigation.openSettings,
}));

describe.each([true, false])("Composer Runtime connection material (floating=%s)", (floating) => {
  beforeEach(() => {
    runtime.snapshot.phase = "reconnecting";
    runtime.snapshot.failure = null;
    navigation.openSettings.mockClear();
  });

  it("explains withdrawn command rights while the exact connection recovers", () => {
    render(
      <FloatingComposer overlayRef={createRef<HTMLDivElement>()} floating={floating}>
        <div>Composer</div>
      </FloatingComposer>,
    );

    expect(screen.getByText("Runtime connection was lost. Reconnecting…")).toBeTruthy();
  });

  it("does not flash a loss notice during cold-start inspection", () => {
    runtime.snapshot.phase = "checking";

    render(
      <FloatingComposer overlayRef={createRef<HTMLDivElement>()} floating={floating}>
        <div>Composer</div>
      </FloatingComposer>,
    );

    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.getByText("Composer")).toBeTruthy();
  });

  it("keeps a failed recovery actionable while automatic retries continue", () => {
    runtime.snapshot.phase = "unavailable";
    runtime.snapshot.failure = { reason: "failed", detail: "connection refused" };

    render(
      <FloatingComposer overlayRef={createRef<HTMLDivElement>()} floating={floating}>
        <div>Composer</div>
      </FloatingComposer>,
    );

    expect(screen.getByText("Runtime is unavailable. Flame will keep trying.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Connection settings" }));
    expect(navigation.openSettings).toHaveBeenCalledWith("connection");
  });
});

it("shows the jump control only while floating without replacing its content", () => {
  const overlay = createRef<HTMLDivElement>();
  const view = render(
    <FloatingComposer overlayRef={overlay} floating>
      <input aria-label="Authoring input" defaultValue="Keep this input" />
    </FloatingComposer>,
  );
  const input = screen.getByRole("textbox", { name: "Authoring input" });
  const originalOverlay = overlay.current;
  expect(screen.getByRole("button", { name: "Jump to latest message" })).toBeTruthy();

  view.rerender(
    <FloatingComposer overlayRef={overlay} floating={false}>
      <input aria-label="Authoring input" defaultValue="Keep this input" />
    </FloatingComposer>,
  );

  expect(screen.queryByRole("button", { name: "Jump to latest message" })).toBeNull();
  expect(screen.getByRole("textbox", { name: "Authoring input" })).toBe(input);
  expect(overlay.current).toBe(originalOverlay);
});
