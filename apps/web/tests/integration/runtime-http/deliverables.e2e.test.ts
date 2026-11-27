import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  mutationSettlementIsUnknown,
  type PluginCommandChange,
  type PluginRecoveryResolution,
} from "@flame/runtime-client";
import { METHODS } from "../../../../../plugins/deliverables/contracts/methods";
import { bindDeliverableOperations } from "../../../../../plugins/deliverables/component/operations";
import type { Revision } from "../../../../../plugins/deliverables/component/types";
import { useRuntimeFixture } from "./fixture";
import { faultUnaryResponse } from "./transportFaults";

describe("Deliverables commands over Runtime HTTP", () => {
  const fixture = useRuntimeFixture("deliverables");

  it("delivers the committed revision to the original matching view after recovery", async () => {
    let installed = (await fixture.client.plugins.list()).data.find(
      ({ selected }) => selected.name === "flame.deliverables",
    );
    if (!installed) {
      const receipt = await fixture.client.plugins.install({
        source: join(fixture.environmentRoot, "plugins", "deliverables"),
      });
      installed = await fixture.getInstallation(receipt.installationId);
    }
    if (!installed.enabled) {
      await fixture.client.plugins.setEnablement({ installationId: installed.id, enabled: true });
      installed = await fixture.getInstallation(installed.id);
    }
    const contribution = installed.selected.contributions.find(({ id }) => id === "deliverables");
    if (!contribution) throw new Error("installed archive has no command contribution");

    const fault = faultUnaryResponse("plugins.call", "afterCommit");
    let disconnected = true;
    const client = fixture.createRuntimeClient((url, init) => {
      if (!disconnected) return fetch(url, init);
      if (fault.attempts() !== 0) throw new Error("the reply remains unavailable");
      return fault.fetch(url, init);
    });
    const lifetime = new AbortController();
    const settlements: PluginRecoveryResolution<Revision>[] = [];
    const unrelated: unknown[] = [];
    const announcements: PluginCommandChange[] = [];
    const stop = client.plugins.subscribeCommands((change) => {
      if (change.type === "resolved") announcements.push(change);
    });
    try {
      const view = bindDeliverableOperations({
        binding: {
          installationId: installed.id,
          digest: installed.selected.digest,
          contribution: contribution.id,
          contract: contribution.contract,
        },
        runtime: client,
        signal: lifetime.signal,
        track: (operation) => operation(lifetime.signal),
      });
      const title = "Receipt belongs to the original Deliverables view";
      const command = view.command(METHODS.create, {
        title,
        format: "docx",
        contentJSON: JSON.stringify({
          title,
          blocks: [{ type: "paragraph", runs: [{ text: "Committed once" }] }],
        }),
        provenance: { sources: [], assumptions: [], units: "", reproduction: "" },
      });
      const id = command.idempotencyKey;
      view.observeRecovery(METHODS.create, id, (resolution) => settlements.push(resolution));
      view.observeRecovery(METHODS.copy, id, (resolution) => unrelated.push(resolution));
      await expect(command).rejects.toSatisfy(mutationSettlementIsUnknown);
      expect(settlements).toEqual([]);
      disconnected = false;
      const original = client.plugins
        .commands()
        .find(({ idempotencyKey }) => idempotencyKey === id);
      if (original?.state !== "recoverable")
        throw new Error("the original create was not retained");

      const decoderFailure = new Error("the recovering caller cannot read the result");
      await expect(
        original.recover(() => {
          throw decoderFailure;
        }),
      ).rejects.toBe(decoderFailure);
      const [resolution] = settlements;
      expect(settlements).toHaveLength(1);
      if (resolution?.type !== "confirmed")
        throw new Error("the originating view did not receive confirmation");
      const retained = await view.query(METHODS.list, { limit: 10 });
      const documents = retained.entries.filter((entry) => entry.title === title);
      expect(documents).toMatchObject([
        { id: resolution.output.documentId, head: resolution.output.id },
      ]);
      expect(client.plugins.commands()).toEqual([]);
      expect(unrelated).toEqual([]);
      expect(announcements).toEqual([
        { type: "resolved", idempotencyKey: id, resolution: { type: "confirmed" } },
      ]);
    } finally {
      stop();
      lifetime.abort();
      await client.close();
    }
  }, 30_000);
});
