import { describe, expect, it } from "vitest";
import { type ContentBlock } from "@/features/agent/public/contentBlock";
import { type AgentSessionView, type Message } from "@/features/agent/public/sessionView";
import { EMPTY_AGENT_SESSION_VIEW } from "@/features/agent/public/sessionView";
import {
  projectSteerDelivery,
  projectSteerReceipt,
  reconcileMessageIdentity,
  reconcileSteerMessages,
} from "@/features/agent/application/view/viewMutations";
const time = "2026-06-03T00:00:00Z";

function view(partial: Partial<AgentSessionView> = {}): AgentSessionView {
  return {
    ...EMPTY_AGENT_SESSION_VIEW,
    messages: [],
    pendingInterrupts: [],
    ...partial,
  };
}

function message(id: string, blocks: ContentBlock[] = []): Message {
  return { id, role: "assistant", createdAt: time, runId: "run_1", blocks };
}

describe("view mutations - messages", () => {
  it("uses only snapshot evidence for accepted and applied Steer input", () => {
    const accepted = {
      ...message("accepted"),
      role: "user" as const,
      runId: null,
      steer: { runId: "run_1", receipt: "accepted" as const },
    };
    const previous = view({ messages: [accepted] });
    const active = view({
      messages: [accepted],
      runsById: { run_1: { id: "run_1", status: "running" } as never },
    });
    expect(reconcileSteerMessages(previous, active)).toBe(active);
    const applied = { ...active, messages: [{ ...accepted, runId: "run_1" }] };
    expect(reconcileSteerMessages(previous, applied)).toBe(applied);
    const replaced = {
      ...active,
      messages: [{ ...accepted, id: "same-payload-other-item", steer: undefined, runId: "run_1" }],
    };
    expect(reconcileSteerMessages(previous, replaced)).toBe(replaced);
  });

  it("keeps an accepted unapplied input visible from the terminal snapshot", () => {
    const accepted = {
      ...message("accepted"),
      role: "user" as const,
      runId: null,
      steer: { runId: "run_1", receipt: "accepted" as const },
    };
    const terminal = view({
      messages: [accepted],
      runsById: { run_1: { id: "run_1", status: "finished" } as never },
    });
    expect(reconcileSteerMessages(view({ messages: [accepted] }), terminal)).toBe(terminal);
  });

  it.each(["pending", "unknown"] as const)(
    "does not infer steer rejection from a terminal Run while its receipt is %s",
    (receipt) => {
      const pending = {
        ...message("local"),
        role: "user" as const,
        runId: null,
        steer: { runId: "run_1", delivery: receipt },
      };
      const terminal = view({ runsById: { run_1: { id: "run_1", status: "finished" } as never } });
      expect(reconcileSteerMessages(view({ messages: [pending] }), terminal)).toEqual({
        ...terminal,
        messages: [pending],
      });
    },
  );

  it("marks a late receipt applied when its durable Item survived an authoritative refresh", () => {
    const current = view({ messages: [{ ...message("durable"), role: "user" }] });
    const accepted = reconcileMessageIdentity(current, "missing-local", "durable", {
      runId: "run_1",
      receipt: "accepted",
    });
    expect(accepted.messages[0]?.steer).toEqual({ runId: "run_1", receipt: "accepted" });
  });

  it("projects local delivery observations separately from immutable Runtime receipts", () => {
    const accepted = {
      ...message("item"),
      role: "user" as const,
      runId: null,
      steer: { runId: "run_1", receipt: "accepted" as const },
    };
    const next = projectSteerDelivery(view({ messages: [accepted] }), "item", {
      runId: "run_1",
      delivery: "unknown",
    });
    expect(next.messages[0]?.steer).toEqual({
      runId: "run_1",
      receipt: "accepted",
      delivery: "unknown",
    });
  });

  it.each(["assistant", "other-run", "other-origin"])(
    "rejects Steer association with %s even when identity does not change",
    (conflict) => {
      const item: Message = {
        ...message("item"),
        role: conflict === "assistant" ? "assistant" : "user",
        runId: conflict === "other-run" ? "run_other" : "run_1",
        ...(conflict === "other-origin"
          ? { steer: { runId: "run_other", delivery: "pending" as const } }
          : {}),
      };
      const current = view({ messages: [item] });
      const receipt = { runId: "run_1", receipt: "accepted" as const };
      expect(() => projectSteerReceipt(current, "item", receipt)).toThrow(
        "agent.steer.itemOwnerMismatch",
      );
      expect(() => reconcileMessageIdentity(current, "item", "item", receipt)).toThrow(
        "agent.steer.itemOwnerMismatch",
      );
    },
  );
  it("does not let a late ACK erase non-application proved by the complete Session read", () => {
    const input: Message = {
      id: "item",
      role: "user",
      runId: null,
      blocks: [],
      steer: { runId: "run_1", receipt: "accepted", application: "notApplied" },
    };
    const next = projectSteerReceipt(view({ messages: [input] }), "item", {
      runId: "run_1",
      receipt: "accepted",
    });
    expect(next.messages[0]?.steer).toMatchObject({
      runId: "run_1",
      receipt: "accepted",
      application: "notApplied",
    });
  });
});
