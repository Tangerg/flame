import { createImageExport } from "@/foundation/imageExport";
import { IMAGE_FILENAME_STEM } from "@/app/product";
import { createRuntimeMutationJournalStorage } from "@/features/connection/adapters/runtimeMutationJournalStorage";
import { createStorage } from "@/extensions/storage";
import { onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;

import { runtimeRequestMeta } from "@/app/runtimeProtocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createBuiltinPlugins } from "@/app/builtinPlugins";
import { createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { createBrowserHost } from "@/platform/browserHost";
import * as themePoints from "@/features/theme/public/extensionPoints";
import * as composerPoints from "@/features/composer/public/extensionPoints";
import * as commandPoints from "@/features/command/public/extensionPoints";
import * as workspacePoints from "@/features/workspace/public/views";
import * as settingsPoints from "@/features/settings/public/panes";
import * as navigationPoints from "@/features/navigation/public/workIndex";
import * as layoutPoints from "@/extensions/layout";
import * as chatPoints from "@/features/chat/public/tools/types";
import * as chatRolePoints from "@/features/chat/public/messageRoles";
import * as agentRunPoints from "@/features/agent/public/runOptions";
import * as i18nPoints from "@/features/i18n/public/extensionPoints";
const kernelPoints = {
  ...themePoints,
  ...composerPoints,
  ...commandPoints,
  ...workspacePoints,
  ...settingsPoints,
  ...navigationPoints,
  ...layoutPoints,
  ...chatPoints,
  ...chatRolePoints,
  ...agentRunPoints,
  ...i18nPoints,
};
import { publishedKernel } from "@/extensions/kernel";
import { lookupDataProvider } from "@/extensions/dataProvider";
import { MCP_SERVERS_KEY } from "@/features/mcp/application/mcpServerQueries";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { type Host } from "dougong";
let running: Host | undefined;
const resetKernelForTest = async () => {
  if (running) await stopKernel(running);
  running = undefined;
};

import { type FlameClient } from "@flame/runtime-client";
let connection: ReturnType<typeof createRuntimeConnection>;

const host = createBrowserHost();

let builtinPlugins: ReturnType<typeof createBuiltinPlugins>;

beforeEach(async () => {
  connection = createRuntimeConnection(
    host,
    runtimeRequestMeta,
    createRuntimeMutationJournalStorage(createStorage("flame.builtin.runtime")),
  );
  await connection.initialize();
  const owned = createTestClient({ connection, host, start: false });
  testClient = owned;
  finishTestClient(() => owned.dispose());
  builtinPlugins = createBuiltinPlugins(
    connection,
    createImageExport(IMAGE_FILENAME_STEM, host.saveFile),
    testClient.navigation,
    testClient.agent,
    testClient.composer,
    testClient.workspace,
    testClient.directory,
    testClient.notifications,
    testClient.installedPlugins,
    testClient.theme,
    testClient.providers,
    testClient.mcpServers,
    testClient.goals,
    testClient.hookTrust,
    testClient.feedback,
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

describe("built-in plugin manifest", () => {
  it("declares every plugin identity exactly once", () => {
    const names = builtinPlugins.map((plugin) => plugin.name);
    expect(new Set(names).size).toBe(names.length);
  });

  it("keeps cumulative context telemetry out of the title bar", () => {
    const names = builtinPlugins.map((plugin) => plugin.name);
    expect(names).not.toContain("flame.builtin.session-usage");
  });
});

describe("built-in contributions", () => {
  afterEach(async () => {
    await resetKernelForTest();
    await connection.dispose();
  });

  it("never have two built-ins under one key on a single-keyed point", async () => {
    running = await startKernel(builtinPlugins);
    const host = publishedKernel();
    expect(host, "the test kernel published nothing").toBeDefined();

    const shadowed: string[] = [];
    let contributions = 0;
    for (const [name, candidate] of Object.entries(kernelPoints)) {
      const point = candidate as { id?: string; token?: unknown; keying?: string };
      if (!point.token || !point.id || point.keying === "multi") continue;
      const raw = [
        ...(host!
          .contributions(point.token as never)
          .get()
          .values() as Iterable<{
          key: string;
          plugin: string;
        }>),
      ];
      contributions += raw.length;
      const owners = new Map<string, string[]>();
      for (const entry of raw)
        owners.set(entry.key, [...(owners.get(entry.key) ?? []), entry.plugin]);
      for (const [key, plugins] of owners)
        if (plugins.length > 1) shadowed.push(`${name}[${key}] <- ${plugins.join(", ")}`);
    }

    expect(contributions).toBeGreaterThan(100);
    expect(shadowed).toEqual([]);
  });

  it("supplies registered plugin reads with the connection's already journal-backed client", async () => {
    const earlyClient = connection.client();
    const received: FlameClient[] = [];
    vi.spyOn(connection, "client").mockImplementation(() => {
      received.push(earlyClient);
      return earlyClient;
    });
    builtinPlugins = createBuiltinPlugins(
      connection,
      createImageExport(IMAGE_FILENAME_STEM, host.saveFile),
      testClient.navigation,
      testClient.agent,
      testClient.composer,
      testClient.workspace,
      testClient.directory,
      testClient.notifications,
      testClient.installedPlugins,
      testClient.theme,
      testClient.providers,
      testClient.mcpServers,
      testClient.goals,
      testClient.hookTrust,
      testClient.feedback,
    );
    running = await startKernel(builtinPlugins);
    expect(received).toEqual([]);
    const failure = new Error("expected read failure");
    vi.spyOn(earlyClient.mcp, "list").mockRejectedValue(failure);
    const fetch = lookupDataProvider(MCP_SERVERS_KEY);
    expect(fetch).toBeDefined();
    await expect(fetch!()).rejects.toBe(failure);
    expect(received.length).toBeGreaterThan(0);
    expect(received.every((client) => client === earlyClient)).toBe(true);
  });
});
