import { describe, expect, it } from "vitest";
import { createFlameClient } from "@flame/runtime-contract/client";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { VISUAL_MODELS, VISUAL_PROVIDERS } from "./runtimeSnapshots";

describe("visual Runtime transport", () => {
  it("answers Settings reads through the validated Runtime client", async () => {
    const client = createFlameClient(createVisualRuntimeTransport());
    try {
      expect((await client.providers.list()).data).toEqual(VISUAL_PROVIDERS);
      expect((await client.models.list("openai")).data).toEqual(
        VISUAL_MODELS.filter((model) => model.provider === "openai"),
      );
      expect((await client.models.list()).data).toEqual(VISUAL_MODELS);
      expect(await client.models.getUtilityRole()).toEqual({
        provider: VISUAL_MODELS[0]!.provider,
        model: VISUAL_MODELS[0]!.id,
      });
      expect(await client.models.getEmbeddingRole()).toEqual({});
      expect((await client.mcp.list()).data).toEqual([]);
      expect((await client.plugins.list()).data).toEqual([]);
      const workspace = await client.workspaces.open({ path: "/Users/visual/scope" });
      expect(await workspace.hooks.list()).toEqual({ hooks: [], projectTrusted: false });
    } finally {
      await client.close();
    }
  });

  it("keeps the inert package subscription open until its consumer retires", async () => {
    const client = createFlameClient(createVisualRuntimeTransport());
    try {
      const subscription = await client.runtimeEvents.subscribe({ topics: ["plugins.changed"] });
      expect(subscription.result).toEqual({});
      const events = subscription.events[Symbol.asyncIterator]();
      const pending = events.next();
      await events.return?.();
      expect(await pending).toMatchObject({ done: true });
    } finally {
      await client.close();
    }
  });

  it("refuses unmodelled writes instead of leaving them pending or reporting success", async () => {
    const client = createFlameClient(createVisualRuntimeTransport());
    try {
      await expect(client.models.setUtilityRole({})).rejects.toThrow(
        'Visual Runtime has no response for "models.setUtilityRole"',
      );
    } finally {
      await client.close();
    }
  });

  it("respects cancellation and refuses requests after closure", async () => {
    const client = createFlameClient(createVisualRuntimeTransport());
    const controller = new AbortController();
    const cancelled = new Error("visual read cancelled");
    controller.abort(cancelled);
    try {
      await expect(client.providers.list(controller.signal)).rejects.toMatchObject({
        name: "RpcTransportError",
        message: "aborted",
      });
      await client.close();
      await expect(client.providers.list()).rejects.toThrow("client closed");
    } finally {
      await client.close();
    }
  });
});
