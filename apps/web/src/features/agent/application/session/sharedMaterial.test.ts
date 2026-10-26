import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, describe, expect, it } from "vitest";

const disposals: Array<() => void> = [];

afterEach(() => {
  for (const dispose of disposals.splice(0).reverse()) dispose();
});

describe("Session shared material contributors", () => {
  it("projects a bounded context into the winning Agent material value", () => {
    disposals.push(
      testClient.agent.access.sharedMaterial.register<{ revision: number }>(
        "companion",
        (sessionId, value) => ({
          sessionId,
          revision: value.revision,
        }),
      ),
    );
    const project = testClient.agent.access.sharedMaterial.stage("ses_1", { revision: 4 });

    expect(project({ plan: { revision: 2 } })).toEqual({
      plan: { revision: 2 },
      companion: { sessionId: "ses_1", revision: 4 },
    });
  });

  it("withdraws a staged value when its plugin owner retires before commit", () => {
    const dispose = testClient.agent.access.sharedMaterial.register<{ revision: number }>(
      "companion",
      (_sessionId, value) => value,
    );
    const project = testClient.agent.access.sharedMaterial.stage("ses_1", { revision: 4 });

    dispose();

    expect(project({ plan: { revision: 2 } })).toEqual({ plan: { revision: 2 } });
  });

  it("rejects a second owner for the same material key", () => {
    disposals.push(testClient.agent.access.sharedMaterial.register("companion", () => null));

    expect(() => testClient.agent.access.sharedMaterial.register("companion", () => null)).toThrow(
      'Agent Session shared material "companion" already has an owner',
    );
  });
});
