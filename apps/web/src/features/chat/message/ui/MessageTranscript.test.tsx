import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { type BlockCtx } from "@/features/chat/message/ui/blockContext";
import { MessageTranscript } from "@/features/chat/message/ui/MessageTranscript";

vi.mock("@/features/chat/message/ui/MessageTurn", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/chat/message/ui/MessageTurn")>();
  return {
    MessageTurn: (props: Parameters<typeof actual.MessageTurn>[0]) => {
      if (props.row.message.id === "render-boom") throw new Error("render-boom");
      return <actual.MessageTurn {...props} />;
    },
  };
});

const CTX: BlockCtx = {
  sessionId: "session",
  expandedIds: new Set(),
  onToggleExpand: () => {},
  textReveal: "instant",
};

function row(id: string, createdAt?: string): TranscriptRow {
  return {
    message: { id, createdAt, runId: "run", role: "assistant", blocks: [] },
    runOwner: { kind: "owned", runId: "run", status: "finished" },
    facts: { toolCalls: {}, delegatedRuns: {}, awaiting: new Map<string, string>() },
  };
}

function transcript(rows: readonly TranscriptRow[]) {
  return <MessageTranscript rows={rows} ctx={CTX} sessionId="session" isRunning={false} />;
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-23T12:00:00"));
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("shared message transcript", () => {
  it("contains a turn that fails to render without blanking its siblings", async () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const healthy = row("healthy-message");
    healthy.message.blocks = [
      { kind: "text", text: "The healthy message remains readable.", status: "complete" },
    ];
    render(transcript([row("render-boom"), healthy]));
    await act(async () => vi.dynamicImportSettled());

    expect(screen.getAllByRole("alert")).toHaveLength(1);
    expect(screen.getByText("The healthy message remains readable.")).toBeTruthy();
  });

  it("separates changes between known days without labeling the first day or undated turns", () => {
    const { container } = render(
      transcript([
        row("undated-first"),
        row("invalid-first", "not-a-date"),
        row("day-one", "2000-01-01T12:00:00"),
        row("undated-middle"),
        row("same-day", "2000-01-01T18:00:00"),
        row("invalid-middle", "not-a-date"),
        row("day-two", "2000-01-02T12:00:00"),
        row("same-day-two", "2000-01-02T18:00:00"),
      ]),
    );

    expect(screen.queryByText("Jan 1, 2000")).toBeNull();
    const separator = screen.getByText("Jan 2, 2000");
    const before = container.querySelector('[data-turn-id="invalid-middle"]')!;
    const after = container.querySelector('[data-turn-id="day-two"]')!;
    expect(before.compareDocumentPosition(separator) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(separator.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
  });

  it("updates day breaks around inserted neighbors while preserving an opened thought", async () => {
    const thought = row("thought", "2000-01-02T12:00:00");
    thought.message.blocks = [
      {
        kind: "reasoning",
        reasoningId: "rationale",
        text: "Inspect module ownership",
        status: "complete",
      },
    ];
    const earlier = row("earlier", "2000-01-01T12:00:00");
    const { rerender } = render(transcript([thought]));
    const trigger = screen.getByRole("button", { name: "Thought" });
    fireEvent.click(trigger);
    await act(async () => vi.dynamicImportSettled());
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.queryByText("Jan 2, 2000")).toBeNull();

    rerender(transcript([earlier, thought]));
    expect(screen.getByText("Jan 2, 2000")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Thought" })).toBe(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");

    rerender(transcript([thought]));
    expect(screen.queryByText("Jan 2, 2000")).toBeNull();
    expect(screen.getByRole("button", { name: "Thought" })).toBe(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Inspect module ownership")).toBeTruthy();
  });

  it("groups completed process steps only when the following answer belongs to the same run", async () => {
    const process = row("process");
    process.message.phase = "commentary";
    process.message.blocks = [
      { kind: "reasoning", reasoningId: "imports", text: "Inspect imports", status: "complete" },
      { kind: "reasoning", reasoningId: "exports", text: "Review exports", status: "complete" },
    ];
    const answer = row("answer");
    answer.message.phase = "finalAnswer";
    answer.message.blocks = [
      { kind: "text", text: "The module is consistent.", status: "complete" },
    ];
    const otherAnswer: TranscriptRow = {
      ...answer,
      message: { ...answer.message, runId: "other-run" },
      runOwner: { kind: "owned", runId: "other-run", status: "finished" },
    };
    const { rerender } = render(transcript([process, otherAnswer]));
    await act(async () => vi.dynamicImportSettled());
    expect(screen.getAllByRole("button", { name: "Thought" })).toHaveLength(2);
    expect(screen.queryByRole("button", { name: "2 steps" })).toBeNull();

    rerender(transcript([process, answer]));
    expect(screen.getByRole("button", { name: "2 steps" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Thought" })).toBeNull();
    expect(screen.getByText("The module is consistent.")).toBeTruthy();
  });
});
