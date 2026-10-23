import { type PropsWithChildren } from "react";
import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { type BlockCtx } from "@/features/chat/public/message/rendering";
import { ContentVisibility } from "@/ui/index";
import { MessageStream } from "@/features/shell/workbench/panel/MessageStream";
const { root, follow } = vi.hoisted(() => {
  const followRef = () => {
    const ref = (element: HTMLDivElement | null) => {
      ref.current = element;
    };
    ref.current = null as HTMLDivElement | null;
    return ref;
  };
  return {
    root: {
      current: {
        running: true,
        terminalTurnIndex: (): number => -1,
      },
    },
    follow: { scrollRef: followRef(), contentRef: followRef() },
  };
});

vi.mock("@/features/agent/public/run", () => ({
  useCurrentRootMaterial: () => root.current,
}));

vi.mock("@/features/chat/public/message/rendering", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/chat/public/message/rendering")>();
  return {
    ...actual,
    RootRunOutcome: () => <div data-testid="root-run-outcome" />,
  };
});

vi.mock("@/extensions/host/Slot", () => ({
  Slot: ({ name }: { name: string }) => <div data-testid={name} />,
}));

vi.mock("use-stick-to-bottom", () => {
  const context = {
    isAtBottom: true,
    scrollToBottom: vi.fn(),
    stopScroll: vi.fn(),
    state: { isAtBottom: true, targetScrollTop: 0 },
    ...follow,
  };
  const StickToBottom = ({ children }: PropsWithChildren) => <div>{children}</div>;
  return {
    StickToBottom,
    useStickToBottomContext: () => context,
  };
});

const CTX: BlockCtx = {
  sessionId: "ses-1",
  expandedIds: new Set(),
  onToggleExpand: vi.fn(),
  textReveal: "smooth",
};

function transcriptRow(status: "running" | "complete"): TranscriptRow {
  const runStatus = status === "running" ? "running" : "finished";
  return {
    message: {
      id: "assistant-terminal-footer",
      runId: "run-terminal-footer",
      role: "assistant",
      blocks: [
        {
          kind: "text",
          itemId: "answer-terminal-footer",
          text: "A long terminal answer whose visible projection still has a reveal backlog.",
          status,
        },
      ],
    },
    runOwner: { kind: "owned", runId: "run-terminal-footer", status: runStatus },
    facts: { toolCalls: {}, delegatedRuns: {}, awaiting: new Map<string, string>() },
  };
}

function optimisticUserRow(): TranscriptRow {
  return {
    message: {
      id: "local-successor-user",
      runId: null,
      role: "user",
      blocks: [
        {
          kind: "text",
          itemId: "local-successor-text",
          text: "Start another run",
          status: "complete",
        },
      ],
    },
    runOwner: { kind: "unassigned" },
    facts: { toolCalls: {}, delegatedRuns: {}, awaiting: new Map<string, string>() },
  };
}

describe("MessageStream terminal footer materialization", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("data-motion");
    root.current = { running: true, terminalTurnIndex: () => -1 };
  });

  it("animates a newly appended turn without fading or transforming its surface", () => {
    const { container, rerender } = render(
      <MessageStream rows={[]} ctx={CTX} sessionId="session-arrival" />,
    );
    rerender(
      <MessageStream rows={[transcriptRow("running")]} ctx={CTX} sessionId="session-arrival" />,
    );
    const turn = container.querySelector<HTMLElement>("[data-turn-id]");
    expect(turn).not.toBeNull();
    const style = getComputedStyle(turn!);
    expect(Number(style.opacity || "1")).toBe(1);
    expect(style.transform || "none").toBe("none");
    expect(turn!.style.top).toBe("6px");
  });

  it("keeps the Run outcome out of layout until the terminal visible generation settles", async () => {
    root.current = { running: true, terminalTurnIndex: () => -1 };
    const { rerender } = render(
      <MessageStream rows={[transcriptRow("running")]} ctx={CTX} sessionId="session-footer" />,
    );
    expect(screen.queryByTestId("root-run-outcome")).toBeNull();

    // Settle the real lazy renderer before testing its visible reveal generation.
    await act(async () => vi.dynamicImportSettled());
    expect(screen.queryByTestId("root-run-outcome")).toBeNull();

    root.current = { running: false, terminalTurnIndex: () => 0 };
    rerender(
      <MessageStream rows={[transcriptRow("complete")]} ctx={CTX} sessionId="session-footer" />,
    );

    expect(screen.queryByTestId("root-run-outcome")).toBeNull();

    document.documentElement.setAttribute("data-motion", "off");
    rerender(
      <MessageStream rows={[transcriptRow("complete")]} ctx={CTX} sessionId="session-footer" />,
    );

    expect(await screen.findByTestId("root-run-outcome")).toBeTruthy();
  });

  it("keeps a finished Run outcome with its exact turn while a successor user message is unassigned", async () => {
    root.current = { running: false, terminalTurnIndex: () => 0 };

    render(
      <MessageStream
        rows={[transcriptRow("complete"), optimisticUserRow()]}
        ctx={CTX}
        sessionId="session-footer"
      />,
    );

    await act(async () => vi.dynamicImportSettled());

    expect(
      screen.getByTestId("root-run-outcome").closest<HTMLElement>("[data-turn-id]")?.dataset.turnId,
    ).toBe("assistant-terminal-footer");
  });
});

describe("MessageStream reading measurement", () => {
  it("detaches follow measurement while an ancestor host hides the retained transcript", async () => {
    root.current = { running: false, terminalTurnIndex: () => -1 };
    const stream = (visible: boolean) => (
      <ContentVisibility visible={visible}>
        <ContentVisibility visible>
          <MessageStream rows={[transcriptRow("complete")]} ctx={CTX} sessionId="session-hidden" />
        </ContentVisibility>
      </ContentVisibility>
    );
    const { container, rerender } = render(stream(true));
    await act(async () => vi.dynamicImportSettled());
    const viewport = container.querySelector<HTMLElement>(".msg-scroll-viewport")!;
    const content = viewport.firstElementChild as HTMLElement;
    const turn = container.querySelector("[data-turn-id]");
    expect(follow.scrollRef.current).toBe(viewport);
    expect(follow.contentRef.current).toBe(content);

    rerender(stream(false));
    expect(follow.scrollRef.current).toBeNull();
    expect(follow.contentRef.current).toBeNull();
    expect(content.isConnected).toBe(true);
    expect(container.querySelector("[data-turn-id]")).toBe(turn);

    rerender(stream(true));
    expect(follow.scrollRef.current).toBe(viewport);
    expect(follow.contentRef.current).toBe(content);
    expect(container.querySelector("[data-turn-id]")).toBe(turn);
  });
});
