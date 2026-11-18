import { createRuntimeMutationJournalStorage } from "@/features/connection/adapters/runtimeMutationJournalStorage";
import { onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;

import { runtimeRequestMeta } from "@/app/runtimeProtocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createBuiltinPlugins } from "@/app/builtinPlugins";
import { createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { createBrowserHost } from "@/platform/browserHost";
import { COMMAND, SHORTCUT } from "@/features/command/public/extensionPoints";
import { lookupExtensionPoint } from "@/extensions/selectors/extensions";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { type Host } from "dougong";
let running: Host | undefined;
const resetKernelForTest = async () => {
  if (running) await stopKernel(running);
  running = undefined;
};
import { normalizeHotkey } from "@tanstack/hotkeys";
let connection: ReturnType<typeof createRuntimeConnection>;

const host = createBrowserHost();

let builtinPlugins: ReturnType<typeof createBuiltinPlugins>;

beforeEach(async () => {
  connection = createRuntimeConnection(
    host,
    runtimeRequestMeta,
    createRuntimeMutationJournalStorage("flame.builtin.runtime"),
  );
  await connection.initialize();
  const owned = createTestClient({ connection, host, start: false });
  testClient = owned;
  finishTestClient(() => owned.dispose());
  builtinPlugins = createBuiltinPlugins(
    connection,
    testClient.navigation,
    testClient.agent,
    testClient.composer,
    testClient.workspace,
    testClient.directory,
    testClient.notifications,
    testClient.installedPlugins,
    testClient.theme,
    testClient.goals,
    testClient.messages,
  );
  vi.stubGlobal("fetch", () => Promise.reject(new Error("offline in tests")));
  vi.stubGlobal(
    "EventSource",
    class {
      close() {}
      addEventListener() {}
    },
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

afterEach(async () => {
  await resetKernelForTest();
  await connection.dispose();
});

describe("built-in shortcuts", () => {
  it("bind one key each", async () => {
    running = await startKernel(builtinPlugins);

    const bindings = new Map<string, string[]>();
    for (const command of lookupExtensionPoint(COMMAND)) {
      if (command.combo === undefined) continue;
      const key = normalizeHotkey(command.combo);
      bindings.set(key, [...(bindings.get(key) ?? []), `command ${command.id}`]);
    }
    for (const shortcut of lookupExtensionPoint(SHORTCUT)) {
      const key = normalizeHotkey(shortcut.key);
      bindings.set(key, [...(bindings.get(key) ?? []), `shortcut ${shortcut.key}`]);
    }

    const collisions = [...bindings]
      .filter(([, owners]) => owners.length > 1)
      .map(([key, owners]) => `${key} <- ${owners.join(", ")}`);
    expect(collisions).toEqual([]);
  });
});
