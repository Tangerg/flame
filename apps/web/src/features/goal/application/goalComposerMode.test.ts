import { afterEach, describe, expect, it, vi } from "vitest";
import {
  GoalComposerModeOwner,
  type GoalComposerSubmission,
} from "@/features/goal/application/goalComposerMode";
let owner: GoalComposerModeOwner | null = null;

afterEach(() => {
  owner?.dispose();
  owner = null;
});

describe("GoalComposerModeOwner", () => {
  it("refuses settlement from before reset while the same Session starts again", () => {
    owner = new GoalComposerModeOwner();
    owner.activate("ses_a");
    const first = owner.begin("ses_a")!;
    const finish = first.finish;
    owner.reset();
    owner.activate("ses_a");
    owner.begin("ses_a");

    expect(finish(true)).toBe(false);
    expect(owner.snapshot()).toMatchObject({ sessionId: "ses_a", phase: "starting" });
  });

  it("owns its mode without a global installation and survives another instance's disposal", () => {
    owner = new GoalComposerModeOwner();
    const independent = new GoalComposerModeOwner();
    expect(owner.activate("ses_a")).toBe(true);
    expect(independent.activate("ses_b")).toBe(true);
    independent.dispose();
    expect(owner.snapshot()).toMatchObject({ sessionId: "ses_a", phase: "armed" });
    expect(owner.begin("ses_a")).not.toBeNull();
  });

  it("moves one Session from armed through authoritative start settlement", () => {
    owner = new GoalComposerModeOwner();
    const changed = vi.fn();
    owner.subscribe(changed);

    expect(owner.activate("ses_a")).toBe(true);
    expect(owner.snapshot()).toMatchObject({ sessionId: "ses_a", phase: "armed" });
    const submission = owner.begin("ses_a")!;
    expect(owner.snapshot().phase).toBe("starting");
    expect(submission.finish(true)).toBe(true);
    expect(owner.snapshot().phase).toBe("inactive");
    expect(changed).toHaveBeenCalledTimes(3);
  });

  it("keeps a failed authoritative start armed and preserves replacement intent until confirmed", () => {
    owner = new GoalComposerModeOwner();
    let submission: GoalComposerSubmission;
    const start = vi.fn((started: GoalComposerSubmission) => {
      submission = started;
    });

    owner.activate("ses_a");
    expect(owner.requestReplacement("ses_a", "Old goal", start)).toBe(true);
    expect(owner.snapshot()).toMatchObject({
      sessionId: "ses_a",
      phase: "confirming",
      replacedObjective: "Old goal",
    });
    expect(owner.confirmReplacement("ses_a")).toBe(true);
    expect(start).toHaveBeenCalledOnce();
    expect(submission!.finish(false)).toBe(true);
    expect(owner.snapshot().phase).toBe("armed");
  });

  it("retires replacement callbacks when disposed", () => {
    owner = new GoalComposerModeOwner();
    const stale = vi.fn();
    owner.activate("ses_a");
    owner.requestReplacement("ses_a", "Old goal", stale);

    owner.dispose();
    const successor = new GoalComposerModeOwner();

    expect(owner.confirmReplacement("ses_a")).toBe(false);
    expect(stale).not.toHaveBeenCalled();
    expect(successor.activate("ses_b")).toBe(true);
    successor.dispose();
  });
});
