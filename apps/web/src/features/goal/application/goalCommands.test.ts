import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  type GoalCommandReceipt,
  type GoalCommandsGateway,
} from "@/features/goal/application/ports/goalCommandsGateway";
import {
  GoalCommandOwner,
  GoalCommandSessionMismatchError,
  type GoalProjectionRepair,
  type GoalCommands,
} from "@/features/goal/application/goalCommands";
let commands: GoalCommands;
let restoreGateway: (() => void) | undefined;

function installGoalCommandOwnerForTest(
  gateway: Partial<GoalCommandsGateway>,
  repairProjection: GoalProjectionRepair = vi.fn().mockResolvedValue(true),
): () => void {
  const owner = new GoalCommandOwner(goalGateway(gateway), repairProjection);
  commands = owner.capture()!;
  return () => owner.dispose();
}

afterEach(() => {
  restoreGateway?.();
  restoreGateway = undefined;
  vi.restoreAllMocks();
});

const receipt = { sessionId: "ses_goal" };

function goalGateway(overrides: Partial<GoalCommandsGateway> = {}): GoalCommandsGateway {
  return {
    start: vi.fn().mockResolvedValue(receipt),
    update: vi.fn().mockResolvedValue(receipt),
    clear: vi.fn().mockResolvedValue(receipt),
    stop: vi.fn().mockResolvedValue(receipt),
    resume: vi.fn().mockResolvedValue(receipt),
    ...overrides,
  };
}

describe("Goal lifecycle commands", () => {
  it("rejects a captured stop command after replacement without targeting the successor", async () => {
    const stop = vi.fn().mockResolvedValue(receipt);
    const successorStop = vi.fn().mockResolvedValue(receipt);
    const owner = new GoalCommandOwner(goalGateway({ stop }), vi.fn().mockResolvedValue(true));
    restoreGateway = () => owner.dispose();
    const capturedCommands = owner.capture()!;
    const captured = capturedCommands.stop.bind(capturedCommands);
    owner.replaceRuntimeGeneration(() => goalGateway({ stop: successorStop }));

    await expect(captured("ses_goal")).rejects.toMatchObject({
      message: "goal_command_generation_retired",
    });
    expect(stop).not.toHaveBeenCalled();
    expect(successorStop).not.toHaveBeenCalled();
  });

  it.each([
    {
      name: "start",
      run: () => commands.start({ sessionId: "ses_goal", objective: "ship it" }),
      gateway: { start: vi.fn().mockRejectedValue(new Error("start response lost")) },
    },
    {
      name: "stop",
      run: () => commands.stop("ses_goal"),
      gateway: { stop: vi.fn().mockRejectedValue(new Error("stop response lost")) },
    },
    {
      name: "update",
      run: () => commands.update({ sessionId: "ses_goal", objective: "ship it better" }),
      gateway: { update: vi.fn().mockRejectedValue(new Error("update response lost")) },
    },
    {
      name: "clear",
      run: () => commands.clear("ses_goal"),
      gateway: { clear: vi.fn().mockRejectedValue(new Error("clear response lost")) },
    },
    {
      name: "resume",
      run: () => commands.resume("ses_goal"),
      gateway: { resume: vi.fn().mockRejectedValue(new Error("resume response lost")) },
    },
  ])("revalidates authoritative Goal state when $name settlement is ambiguous", async (test) => {
    const repairProjection = vi.fn().mockResolvedValue(true);
    restoreGateway = installGoalCommandOwnerForTest(
      {
        start: vi.fn().mockResolvedValue(receipt),
        stop: vi.fn().mockResolvedValue(receipt),
        resume: vi.fn().mockResolvedValue(receipt),
        ...test.gateway,
      },
      repairProjection,
    );

    await expect(test.run()).rejects.toThrow("response lost");

    expect(repairProjection).toHaveBeenCalledWith("ses_goal");
  });

  it("preserves the command failure when authoritative revalidation also fails", async () => {
    const commandError = new Error("stop response lost");
    restoreGateway = installGoalCommandOwnerForTest(
      {
        start: vi.fn().mockResolvedValue(receipt),
        stop: vi.fn().mockRejectedValue(commandError),
        resume: vi.fn().mockResolvedValue(receipt),
      },
      vi.fn().mockRejectedValue(new Error("snapshot unavailable")),
    );

    await expect(commands.stop("ses_goal")).rejects.toBe(commandError);
  });

  it("does not turn an accepted Goal command into a failure when projection repair fails", async () => {
    restoreGateway = installGoalCommandOwnerForTest(
      {
        start: vi.fn().mockResolvedValue(receipt),
        stop: vi.fn().mockResolvedValue(receipt),
        resume: vi.fn().mockResolvedValue(receipt),
      },
      vi.fn().mockRejectedValue(new Error("snapshot unavailable")),
    );

    await expect(commands.stop("ses_goal")).resolves.toBeUndefined();
  });

  it("repairs standing state only through the mounted Session material owner", async () => {
    const repairProjection = vi.fn().mockResolvedValue(true);
    restoreGateway = installGoalCommandOwnerForTest(
      {
        start: vi.fn().mockResolvedValue(receipt),
        stop: vi.fn().mockResolvedValue(receipt),
        resume: vi.fn().mockResolvedValue(receipt),
      },
      repairProjection,
    );

    await commands.stop("ses_goal");

    expect(repairProjection).toHaveBeenCalledWith("ses_goal");
  });

  it("serializes lifecycle commands for one Session while leaving intent order intact", async () => {
    const stopping = Promise.withResolvers<GoalCommandReceipt>();
    const stop = vi.fn(() => stopping.promise);
    const resume = vi.fn().mockResolvedValue(receipt);
    restoreGateway = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop,
      resume,
    });

    const first = commands.stop("ses_goal");
    const second = commands.resume("ses_goal");
    await vi.waitFor(() => expect(stop).toHaveBeenCalledOnce());
    expect(resume).not.toHaveBeenCalled();

    stopping.resolve({ sessionId: "ses_goal" });
    await expect(first).resolves.toBeUndefined();
    await expect(second).resolves.toBeUndefined();
    expect(resume).toHaveBeenCalledOnce();
  });

  it("keeps the next lifecycle command behind authoritative revalidation", async () => {
    const revalidated = Promise.withResolvers<void>();
    const repairProjection = vi.fn(() => revalidated.promise);
    const stop = vi.fn().mockResolvedValue(receipt);
    const resume = vi.fn().mockResolvedValue(receipt);
    restoreGateway = installGoalCommandOwnerForTest(
      {
        start: vi.fn().mockResolvedValue(receipt),
        stop,
        resume,
      },
      repairProjection,
    );

    const first = commands.stop("ses_goal");
    const second = commands.resume("ses_goal");
    await vi.waitFor(() => expect(stop).toHaveBeenCalledOnce());
    expect(resume).not.toHaveBeenCalled();

    revalidated.resolve();
    await expect(first).resolves.toBeUndefined();
    await expect(second).resolves.toBeUndefined();
    expect(resume).toHaveBeenCalledOnce();
  });

  it("retires in-flight and queued Goal commands when their owner is replaced", async () => {
    const stopping = Promise.withResolvers<GoalCommandReceipt>();
    const retiredStop = vi.fn(() => stopping.promise);
    const retiredResume = vi.fn().mockResolvedValue(receipt);
    const disposePredecessor = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop: retiredStop,
      resume: retiredResume,
    });
    restoreGateway = disposePredecessor;

    const inFlight = commands.stop("ses_goal");
    const queued = commands.resume("ses_goal");
    let inFlightOutcome = "pending";
    let queuedOutcome = "pending";
    const observedInFlight = inFlight.then(
      () => {
        inFlightOutcome = "resolved";
      },
      () => {
        inFlightOutcome = "retired";
      },
    );
    const observedQueued = queued.then(
      () => {
        queuedOutcome = "resolved";
      },
      () => {
        queuedOutcome = "retired";
      },
    );
    await vi.waitFor(() => expect(retiredStop).toHaveBeenCalledOnce());

    const successorResume = vi.fn().mockResolvedValue(receipt);
    disposePredecessor();
    const disposeSuccessor = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop: vi.fn().mockResolvedValue(receipt),
      resume: successorResume,
    });
    restoreGateway = () => {
      disposeSuccessor();
      disposePredecessor();
    };
    await vi.waitFor(() => expect(inFlightOutcome).toBe("retired"));
    await vi.waitFor(() => expect(queuedOutcome).toBe("retired"));
    const outcomesAtReplacement = [inFlightOutcome, queuedOutcome];

    stopping.resolve(receipt);
    await Promise.all([observedInFlight, observedQueued]);

    expect(outcomesAtReplacement).toEqual(["retired", "retired"]);
    expect(retiredResume).not.toHaveBeenCalled();
    expect(successorResume).not.toHaveBeenCalled();
  });

  it("gives a Runtime successor a new gateway without lending it queued predecessor intents", async () => {
    const stopping = Promise.withResolvers<GoalCommandReceipt>();
    const retiredStop = vi.fn(() => stopping.promise);
    const retiredResume = vi.fn().mockResolvedValue(receipt);
    const owner = new GoalCommandOwner(
      goalGateway({
        start: vi.fn().mockResolvedValue(receipt),
        stop: retiredStop,
        resume: retiredResume,
      }),
      vi.fn().mockResolvedValue(true),
    );
    restoreGateway = () => owner.dispose();
    commands = owner.capture()!;

    const inFlight = commands.stop("ses_goal").catch((error: unknown) => error);
    const queued = commands.resume("ses_goal").catch((error: unknown) => error);
    await vi.waitFor(() => expect(retiredStop).toHaveBeenCalledOnce());

    const successorResume = vi.fn().mockResolvedValue(receipt);
    expect(
      owner.replaceRuntimeGeneration(() =>
        goalGateway({
          start: vi.fn().mockResolvedValue(receipt),
          stop: vi.fn().mockResolvedValue(receipt),
          resume: successorResume,
        }),
      ),
    ).toBe(true);

    expect(wasGenerationRetired(await inFlight)).toBe(true);
    expect(wasGenerationRetired(await queued)).toBe(true);
    expect(retiredResume).not.toHaveBeenCalled();
    expect(successorResume).not.toHaveBeenCalled();

    await expect(owner.capture()!.resume("ses_goal")).resolves.toBeUndefined();
    expect(successorResume).toHaveBeenCalledOnce();
    stopping.resolve(receipt);
  });

  it("rejects a mutation response addressed to another Session and still revalidates", async () => {
    const repairProjection = vi.fn().mockResolvedValue(true);
    restoreGateway = installGoalCommandOwnerForTest(
      {
        start: vi.fn().mockResolvedValue(receipt),
        stop: vi.fn().mockResolvedValue({ sessionId: "ses_other" }),
        resume: vi.fn().mockResolvedValue(receipt),
      },
      repairProjection,
    );

    const failure = await commands.stop("ses_goal").catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(GoalCommandSessionMismatchError);
    expect(failure).toMatchObject({
      expectedSessionId: "ses_goal",
      actualSessionId: "ses_other",
    });

    expect(repairProjection).toHaveBeenCalledWith("ses_goal");
  });
});
