import { describe, expect, it } from "vitest";
import {
  createFlameClient,
  createHttpTransport,
  mutationSettlementIsUnknown,
} from "@flame/runtime-client";
import {
  HTTP_HEADERS,
  type CallPluginRequest,
  type CallPluginResponse,
} from "@flame/runtime-client/wire";
import { METHODS } from "../../../../../plugins/memory/component/methods";
import { bindMemoryOperations } from "../../../../../plugins/memory/component/memoryOperations";
import type { AddResult } from "../../../../../plugins/memory/package/io.github.tangerg.flame/contracts/methods.generated";
import { useRuntimeFixture } from "./fixture";
import { MemoryMutationStorage } from "./mutationStorage";
import { within } from "./observation";

describe("Memory commands over Runtime HTTP", () => {
  const fixture = useRuntimeFixture("memory");

  it("recovers the original saved Memory after the authoring client closes", async () => {
    const installation = fixture.managedOwners.get("memory")!;
    const component = installation.selected.components.find(({ id }) => id === "memory")!;
    const contribution = installation.selected.contributions.find(
      ({ id }) => id === component.contribution,
    )!;
    const binding = {
      installationId: installation.id,
      digest: installation.selected.digest,
      contribution: contribution.id,
      contract: contribution.contract,
    };
    const storage = new MemoryMutationStorage();
    const originalLifetime = new AbortController();
    const replacementLifetime = new AbortController();
    const committed = Promise.withResolvers<{ key: string; result: AddResult }>();
    const withdrawReply = Promise.withResolvers<void>();
    const delivered: string[] = [];
    const openClient = (signal: AbortSignal) => {
      const runtime = createFlameClient(
        createHttpTransport({
          baseUrl: fixture.baseUrl,
          fetch: async (url, init) => {
            const headers = new Headers(init?.headers);
            if (headers.get(HTTP_HEADERS.method) !== "plugins.call") return fetch(url, init);
            const { params } = JSON.parse(String(init?.body)) as { params: CallPluginRequest };
            if (params.route.method !== METHODS.add) return fetch(url, init);
            const key = headers.get(HTTP_HEADERS.idempotencyKey)!;
            delivered.push(key);
            const response = await fetch(url, init);
            if (delivered.length !== 1) return response;
            const reply = (await response.clone().json()) as { result?: CallPluginResponse };
            if (!reply.result?.output) {
              committed.reject(
                new Error(`Runtime did not save the original Memory: ${JSON.stringify(reply)}`),
              );
              return response;
            }
            committed.resolve({ key, result: JSON.parse(reply.result.output) as AddResult });
            await withdrawReply.promise;
            throw new TypeError("injected loss of the original Memory response");
          },
        }),
        { capabilities: () => fixture.negotiated, mutationStorage: storage },
      );
      return {
        runtime,
        memory: bindMemoryOperations({
          binding,
          runtime,
          signal,
          track: (operation) => operation(signal),
        }),
      };
    };
    const first = openClient(originalLifetime.signal);
    let second: ReturnType<typeof openClient> | undefined;
    let outstanding: Promise<unknown> | undefined;
    try {
      const input = {
        scope: "user" as const,
        cwd: "",
        content: "Keep the captured Memory receipt.",
      };
      const writing = first.memory.submit(METHODS.add, input);
      if (!writing) throw new Error("the original Memory was not submitted");
      outstanding = writing.catch((error: unknown) => error);
      const original = await within(committed.promise, "the original Memory write");
      originalLifetime.abort();
      withdrawReply.resolve();
      expect(mutationSettlementIsUnknown(await outstanding)).toBe(true);
      await first.runtime.close();

      second = openClient(replacementLifetime.signal);
      const pending = second.memory.pendingCommand();
      if (!pending) throw new Error("the replacement client lost the original command");
      expect(pending).toMatchObject({ method: METHODS.add, idempotencyKey: original.key });
      expect(second.memory.submit(METHODS.add, input)).toBeUndefined();
      expect(delivered).toEqual([original.key]);
      await expect(pending.recover()).resolves.toEqual(original.result);
      expect(second.memory.pendingCommand()).toBeUndefined();
      expect(delivered).toEqual([original.key, original.key]);

      const saved = await second.memory.query(METHODS.list, {
        scope: input.scope,
        cwd: "",
        cursor: "",
      });
      expect(saved.rows).toEqual([
        expect.objectContaining({ id: original.result.item.id, content: input.content }),
      ]);
    } finally {
      originalLifetime.abort();
      replacementLifetime.abort();
      withdrawReply.resolve();
      await outstanding;
      await first.runtime.close();
      await second?.runtime.close();
    }
  }, 30_000);
});
