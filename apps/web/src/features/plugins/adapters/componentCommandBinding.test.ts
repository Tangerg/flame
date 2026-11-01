import { describe, expect, it, vi } from "vitest";
import { bindOperations, type ComponentContext } from "@flame/component-sdk";

describe("typed component command recovery", () => {
  it("exposes only the captured release and contract", async () => {
    const target = { scope: "client", reference: "" } as const;
    const binding = { installationId: "notes", contribution: "changes", digest: "release-a" };
    const current = { ...binding, contract: "current", method: "save", target };
    const handle = (key: string, route: typeof current) => ({
      idempotencyKey: key,
      binding: route,
      recover: vi.fn(),
      retire: vi.fn(),
    });
    const handles = [
      handle("intended", current),
      handle("prior", { ...current, digest: "release-b" }),
      handle("different contract", { ...current, contract: "replaced" }),
    ];
    const scope = {
      binding,
      signal: new AbortController().signal,
      runtime: { plugins: { pendingCommands: () => handles } },
    } as unknown as ComponentContext;
    const operations = bindOperations<{ save: object }, { save: string }>(
      scope,
      current.contract,
      target,
    );
    expect(operations.pendingCommands("save").map((item) => item.idempotencyKey)).toEqual([
      "intended",
    ]);
  });
});
