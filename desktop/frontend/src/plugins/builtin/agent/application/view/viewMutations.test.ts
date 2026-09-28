import { describe, expect, it } from "vitest";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { AgentProblem, AgentSessionView, Message } from "@/plugins/sdk/types/agentSessionView";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import {
  dropMessage,
  reconcileMessageIdentity,
  reconcileSteerMessages,
  setCommandError,
} from "./viewMutations";

const time = "2026-06-03T00:00:00Z";

function view(partial: Partial<AgentSessionView> = {}): AgentSessionView {
  return {
    ...EMPTY_AGENT_SESSION_VIEW,
    messages: [],
    timeline: [],
    pendingInterrupts: [],
    ...partial,
  };
}

function message(id: string, blocks: ContentBlock[] = []): Message {
  return { id, role: "assistant", createdAt: time, runId: "run_1", blocks };
}

describe("view mutations - messages", () => {
  it("preserves an accepted steer through a live snapshot and applies only its exact durable Item", () => {
    const pending = {
      ...message("accepted"),
      role: "user" as const,
      runId: null,
      steer: { runId: "run_1", status: "accepted" as const },
    };
    const previous = view({ messages: [pending] });
    const active = view({ runsById: { run_1: { id: "run_1", status: "running" } as never } });
    const retained = reconcileSteerMessages(previous, active);
    expect(retained).toEqual({ view: { ...active, messages: [pending] }, unapplied: false });
    const applied = { ...active, messages: [{ ...pending, steer: undefined, runId: "run_1" }] };
    expect(reconcileSteerMessages(previous, applied)).toMatchObject({
      unapplied: false,
      view: { messages: [{ id: "accepted", runId: "run_1", steer: { status: "applied" } }] },
    });
    const unrelated = {
      ...active,
      messages: [{ ...pending, id: "same-payload-other-item", steer: undefined, runId: "run_1" }],
    };
    expect(reconcileSteerMessages(previous, unrelated).view.messages.map(({ id }) => id)).toEqual([
      "same-payload-other-item",
      "accepted",
    ]);
  });

  it("removes an unapplied steer only after a full terminal snapshot and reports it once", () => {
    const previous = view({
      messages: [
        { ...message("accepted"), runId: null, steer: { runId: "run_1", status: "accepted" } },
      ],
    });
    const terminal = view({ runsById: { run_1: { id: "run_1", status: "finished" } as never } });
    const settled = reconcileSteerMessages(previous, terminal);
    expect(settled.view.messages).toEqual([]);
    expect(settled.unapplied).toBe(true);
    expect(reconcileSteerMessages(settled.view, terminal).unapplied).toBe(false);
  });

  it("marks a late receipt applied when its durable Item survived an authoritative refresh", () => {
    const current = view({ messages: [{ ...message("durable"), role: "user" }] });
    const accepted = reconcileMessageIdentity(current, "missing-local", "durable", "run_1");
    expect(accepted.messages[0]?.steer).toEqual({ runId: "run_1", status: "applied" });
  });

  it("relabels an optimistic message without touching unrelated messages", () => {
    const original = view({
      messages: [message("local-1"), message("assistant-1")],
    });

    const next = reconcileMessageIdentity(original, "local-1", "server-1");

    expect(next.messages.map((m) => m.id)).toEqual(["server-1", "assistant-1"]);
    expect(next.messages[1]).toBe(original.messages[1]);
  });

  it("collapses a provisional message when the durable target won the race", () => {
    const original = view({
      messages: [message("local-1"), message("server-1")],
    });

    const next = reconcileMessageIdentity(original, "local-1", "server-1");

    expect(next.messages.map((message) => message.id)).toEqual(["server-1"]);
    expect(next.messages[0]).toBe(original.messages[1]);
  });

  it("leaves missing and identical identities unchanged", () => {
    const original = view({ messages: [message("local-1")] });

    expect(reconcileMessageIdentity(original, "missing", "server-2")).toBe(original);
    expect(reconcileMessageIdentity(original, "local-1", "local-1")).toBe(original);
  });

  it("drops a message by id and leaves unknown ids as no-ops", () => {
    const original = view({
      messages: [message("m1"), message("m2")],
    });

    const next = dropMessage(original, "m1");

    expect(next.messages.map((m) => m.id)).toEqual(["m2"]);
    expect(dropMessage(original, "missing")).toBe(original);
  });
});

describe("view mutations - run state", () => {
  it("sets and clears a command error only when the value changes", () => {
    const error: AgentProblem = { message: "boom", code: "provider_error" };
    const original = view({ commandError: error });

    expect(setCommandError(original, error)).toBe(original);
    expect(setCommandError(original, null)).toMatchObject({ commandError: null });
  });
});
