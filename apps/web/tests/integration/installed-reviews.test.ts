import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { parseJSON } from "@frame/sdk/json";
import { createFlameClient } from "@flame/runtime-client";
import { readFileSync } from "node:fs";
import path from "node:path";
import { createMemoryTransport } from "@flame/runtime-client/testing";

import { bindOperations, type ComponentContext } from "@flame/component-sdk";

import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../../../../examples/plugins/reviews/component/methods.generated";
import { ANALYZERS_JSON } from "../../../../examples/plugins/reviews/component/board.generated";
let transport: ReturnType<typeof createMemoryTransport>;

let client: ReturnType<typeof createFlameClient>;

let owner: AbortController;

let context: ComponentContext;

beforeEach(() => {
  transport = createMemoryTransport();
  client = createFlameClient(transport);
  owner = new AbortController();
  context = {
    component: {
      installationId: "e4407303-68e3-495d-86d6-9c3f575a51ed",
      digest: "a".repeat(64),
      componentId: "test",
    },
    saveFile() {
      throw new Error("This fixture must not save files");
    },
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose() {
      throw new Error("Composition is not used by this fixture");
    },
    runtime: client,
    locale: "en",
    signal: owner.signal,

    binding: {
      installationId: "e4407303-68e3-495d-86d6-9c3f575a51ed",
      digest: "a".repeat(64),
      contribution: "review-operations",
      contract: CAPABILITY_JSON,
    },
    telemetry: {
      read: () => ({ spans: [], metrics: [], logs: [] }),
      clear() {},
    },
    track: (operation) => operation(context.signal),
  };
});

afterEach(async () => {
  owner.abort();
  await client.close();
});

describe("installed Reviews contextual actions", () => {
  it("admits component work before the Runtime client can create a command", () => {
    const refused = new Error("component work capacity exhausted");
    const call = vi.spyOn(client.plugins, "call");
    const operations = bindOperations<MethodInputs, MethodOutputs>(
      {
        ...context,
        track() {
          throw refused;
        },
      },
      CAPABILITY_JSON,
      { scope: "installation", reference: context.binding!.installationId },
    );
    expect(() =>
      operations.command("io.github.tangerg.flame.examples/reviews.update", {
        id: "example-review",
        expectedRevision: 1,
        status: "resolved",
      }),
    ).toThrow(refused);
    expect(call).not.toHaveBeenCalled();
    expect(transport.outbox()).toEqual([]);
  });

  it("preserves the generated point contract through the public TypeScript boundary", () => {
    const source = readFileSync(
      path.resolve(
        "../../examples/plugins/reviews/package/io.github.tangerg.flame/contracts/analyzers.generated.json",
      ),
      "utf8",
    );
    expect(parseJSON(ANALYZERS_JSON)).toEqual(parseJSON(source));
  });
});
