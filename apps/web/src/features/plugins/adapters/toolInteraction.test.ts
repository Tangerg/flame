import { describe, expect, it, vi } from "vitest";
import { type FlameClient, type RunRef } from "@flame/runtime-client";
import { type AgentInputSettlement } from "@/features/agent/public/input";
import { captureToolInteraction, type SubmitCapturedAgentInput } from "./toolInteraction";

function fixture(submitInput: SubmitCapturedAgentInput, admit = () => {}) {
  const controller = new AbortController();
  const run: RunRef = {
    id: "run_original",
    sessionId: "ses_original",
    status: "finished",
    provider: "provider",
    model: "model",
    createdAt: "2026-10-27T00:00:00Z",
    metrics: { steps: 1, activeDurationMillis: 1 },
    protocolProfile: { interruptTypes: [], requiredFeatures: [] },
  };
  const get = vi.fn(async () => run);
  const source = { runId: run.id, itemId: "item_original" };
  const tracked: Promise<unknown>[] = [];
  const interaction = captureToolInteraction(
    { runs: { get } } as unknown as FlameClient,
    submitInput,
    source,
    controller.signal,
    (factory) => {
      admit();
      const operation = factory(controller.signal);
      tracked.push(operation);
      return operation;
    },
  );
  return { interaction, source, controller, get, tracked };
}

const accepted = {
  status: "accepted" as const,
  sessionId: "ses_original",
  runId: "run_followup",
  userItemId: "item_followup",
};

describe("captured tool interaction", () => {
  it("admits the original lifetime before starting I/O and leaves a denied submission inactive", async () => {
    const submit = vi.fn<SubmitCapturedAgentInput>(async () => accepted);
    const admit = vi.fn<() => void>(() => {
      throw new Error("original lifetime unavailable");
    });
    const { interaction, get } = fixture(submit, admit);
    const input = { action: "Continue", payloadJSON: "{}" };
    expect(() => interaction.submit(input)).toThrow("original lifetime unavailable");
    expect(get).not.toHaveBeenCalled();
    expect(submit).not.toHaveBeenCalled();
    admit.mockImplementation(() => undefined);
    await expect(interaction.submit(input)).resolves.toEqual(accepted);
    expect(get).toHaveBeenCalledOnce();
    expect(submit).toHaveBeenCalledOnce();
  });

  it("admits recovery before observing the original command and retains denied recovery", async () => {
    const recover = vi.fn(async () => accepted);
    const submit = vi.fn<SubmitCapturedAgentInput>(async () => ({
      status: "unknown",
      idempotencyKey: "original-key",
      recover,
    }));
    const admit = vi.fn<() => void>(() => undefined);
    const { interaction } = fixture(submit, admit);
    const result = await interaction.submit({ action: "Continue", payloadJSON: "{}" });
    if (result.status !== "unknown") throw new Error("expected unknown receipt");
    admit.mockImplementation(() => {
      throw new Error("original lifetime unavailable");
    });
    expect(() => result.recover()).toThrow("original lifetime unavailable");
    expect(recover).not.toHaveBeenCalled();
    admit.mockImplementation(() => undefined);
    await expect(result.recover()).resolves.toEqual(accepted);
    expect(recover).toHaveBeenCalledOnce();
    expect(submit).toHaveBeenCalledOnce();
  });

  it("resolves the source Run on submission and retains the exact form JSON in ordinary input", async () => {
    const pending = Promise.withResolvers<AgentInputSettlement>();
    const submit = vi.fn<SubmitCapturedAgentInput>(() => pending.promise);
    const { interaction, source, get, tracked, controller } = fixture(submit);
    source.runId = "run_other_navigation";
    expect(get).not.toHaveBeenCalled();
    expect(Object.isFrozen(interaction.source)).toBe(true);
    const payloadJSON = '{ "amount": 9007199254740993, "scale": 1.00 }';
    const result = interaction.submit({ action: "runs.cancel", payloadJSON });
    const observed = vi.fn();
    void result.then(observed);
    await vi.waitFor(() => expect(submit).toHaveBeenCalledOnce());
    expect(get).toHaveBeenCalledWith("run_original", controller.signal);
    expect(submit).toHaveBeenCalledWith(
      "ses_original",
      {
        parts: [
          {
            kind: "text",
            text: [
              "Tool result interaction",
              'Source Run: "run_original"',
              'Source Item: "item_original"',
              'Action: "runs.cancel"',
              "Form payload (JSON):",
              payloadJSON,
            ].join("\n"),
          },
        ],
      },
      controller.signal,
    );
    expect(observed).not.toHaveBeenCalled();
    expect(tracked).toEqual([result]);
    pending.resolve(accepted);
    await expect(result).resolves.toEqual(accepted);
  });

  it("keeps an unknown command and its original recovery instead of accepting a replacement", async () => {
    const recover = vi.fn(async () => accepted);
    const submit = vi.fn<SubmitCapturedAgentInput>(async () => ({
      status: "unknown",
      idempotencyKey: "original-key",
      recover,
    }));
    const { interaction, get } = fixture(submit);
    const result = await interaction.submit({ action: "Continue", payloadJSON: '{"a":1}' });
    expect(result).toMatchObject({ status: "unknown", idempotencyKey: "original-key" });
    expect(() => interaction.submit({ action: "Changed", payloadJSON: '{"a":2}' })).toThrow(
      /original/,
    );
    if (result.status !== "unknown") throw new Error("expected unknown receipt");
    await expect(result.recover()).resolves.toEqual(accepted);
    expect(recover).toHaveBeenCalledOnce();
    expect(get).toHaveBeenCalledOnce();
    expect(submit).toHaveBeenCalledOnce();
  });

  it("refuses stale components before new I/O and does not turn retirement into acceptance", async () => {
    const submit = vi.fn<SubmitCapturedAgentInput>(async () => ({
      status: "retired",
      sessionId: "ses_original",
      runId: "run_retired",
    }));
    const { interaction, controller, get } = fixture(submit);
    await expect(interaction.submit({ action: "Continue", payloadJSON: "{}" })).resolves.toEqual({
      status: "retired",
      sessionId: "ses_original",
      runId: "run_retired",
    });
    controller.abort(new Error("component retired"));
    expect(() => interaction.submit({ action: "Continue", payloadJSON: "{}" })).toThrow(
      "component retired",
    );
    expect(get).toHaveBeenCalledOnce();
  });

  it("joins an already dispatched input after component closure without canceling its Run", async () => {
    const pending = Promise.withResolvers<AgentInputSettlement>();
    const submit = vi.fn<SubmitCapturedAgentInput>(() => pending.promise);
    const { interaction, controller } = fixture(submit);
    const result = interaction.submit({ action: "Continue", payloadJSON: "{}" });
    await vi.waitFor(() => expect(submit).toHaveBeenCalledOnce());
    controller.abort();
    pending.resolve(accepted);
    await expect(result).resolves.toEqual(accepted);
  });

  it("rejects invalid JSON and simultaneous submissions before opening another input", async () => {
    const pending = Promise.withResolvers<AgentInputSettlement>();
    const submit = vi.fn<SubmitCapturedAgentInput>(() => pending.promise);
    const { interaction, get } = fixture(submit);
    expect(() => interaction.submit({ action: "Continue", payloadJSON: "undefined" })).toThrow(
      /InvalidSymbol/,
    );
    expect(get).not.toHaveBeenCalled();
    const result = interaction.submit({ action: "Continue", payloadJSON: "{}" });
    expect(() => interaction.submit({ action: "Continue", payloadJSON: "{}" })).toThrow(/already/);
    pending.resolve(accepted);
    await result;
    expect(submit).toHaveBeenCalledOnce();
  });
});
