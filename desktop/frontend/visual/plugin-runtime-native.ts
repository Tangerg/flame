import { Call, Events } from "@wailsio/runtime";
import { z } from "zod";
import {
  asSessionId,
  createFlameClient,
  createHttpTransport,
  RpcTransportError,
  type FlameClient,
} from "@flame/runtime-contract/client";
import { runtimeRequestMeta } from "@/main/runtimeProtocol";
import { desktopPluginCarrier } from "@/platform/desktopPluginCarrier";
import { mountTrajectoryFrame } from "@/plugins/builtin/settings/plugins-pane/adapters/trajectoryFrame";
import { createTrajectoryViewReads } from "@/plugins/builtin/settings/plugins-pane/adapters/trajectoryReads";

const FixtureSchema = z.strictObject({
  endpoint: z.url(),
  localToken: z.string().min(1),
  installationId: z.string().min(1),
  digest: z.string().min(1),
  viewId: z.string().min(1),
  sessionId: z.string().min(1),
  workspacePath: z.string().min(1),
  htmlSHA256: z.string().regex(/^[a-f0-9]{64}$/),
});

const carrier = desktopPluginCarrier({
  call: (method, ...args) => Call.ByName(method, ...args),
  on: async (event, listener) => Events.On(event, (value) => listener(value.data)),
});

function expect(value: unknown, reason: string): asserts value {
  if (!value) throw new Error(reason);
}

async function run() {
  const fixture = FixtureSchema.parse(await Call.ByName("main.NativeRuntimeGate.Configuration"));
  const binding = {
    installationId: fixture.installationId,
    digest: fixture.digest,
    viewId: fixture.viewId,
  };
  const sessionId = asSessionId(fixture.sessionId);
  const clients: FlameClient[] = [];
  const pages: (() => Promise<void>)[] = [];
  const failures: unknown[] = [];
  try {
    for (let generation = 0; generation < 2; generation++) {
      const client = createFlameClient(
        createHttpTransport({ baseUrl: fixture.endpoint, localToken: fixture.localToken }),
        { requestMeta: () => runtimeRequestMeta("desktop") },
      );
      clients.push(client);
      await client.runtime.discover();
      const [session, installations, servers, tools] = await Promise.all([
        client.sessions.get(sessionId),
        client.plugins.list(),
        client.mcp.list(),
        client.mcp.listTools({
          name: "reviews",
          origin: { type: "installation", installationId: fixture.installationId },
        }),
      ]);
      expect(
        session.workspace.ref.path === fixture.workspacePath,
        "Runtime changed the workspace path",
      );
      const installation = installations.data.find((value) => value.id === fixture.installationId);
      expect(
        installation?.selected.digest === fixture.digest &&
          installation.presentation === "admitted",
        "Runtime did not admit the selected plugin view",
      );
      expect(
        servers.data.some(
          (value) =>
            value.id.origin.type === "installation" &&
            value.id.origin.installationId === fixture.installationId &&
            value.id.name === "reviews" &&
            value.status.type === "connected" &&
            value.status.toolCount === 2,
        ),
        "Runtime did not activate the same installation's Go backend",
      );
      expect(
        tools.data.some((value) => value.name === "list_reviews") &&
          tools.data.some((value) => value.name === "update_review"),
        "Runtime did not discover the review backend's tools",
      );
      const connected = Promise.withResolvers<void>();
      const controller = new AbortController();
      const container = document.getElementById("page");
      expect(container, "native gate container is missing");
      const reads = createTrajectoryViewReads(client.plugins, binding, sessionId);
      const close = mountTrajectoryFrame({
        container,
        carrier,
        signal: controller.signal,
        reads: {
          async load(signal) {
            const result = await reads.load(signal);
            const digest = new Uint8Array(
              await crypto.subtle.digest("SHA-256", new TextEncoder().encode(result.html)),
            );
            expect(
              Array.from(digest, (byte) => byte.toString(16).padStart(2, "0")).join("") ===
                fixture.htmlSHA256,
              "Runtime returned different plugin HTML",
            );
            expect(
              !result.html.includes(fixture.localToken),
              "Runtime exposed the local token to the guest",
            );
            return result;
          },
          read: reads.read,
        },
        status: (value) => {
          if (value.type === "ready") connected.resolve();
          if (value.type === "failure") connected.reject(new Error(value.reason));
        },
      });
      pages.push(close);
      await connected.promise;
      if (generation === 1) {
        await pages[0]!();
        await clients[0]!.close();
        expect(
          (await Call.ByName("main.NativeRuntimeGate.ActivePageCount")) === 1,
          "predecessor cleanup removed the successor page",
        );
        await client.plugins.readTrajectory({ ...binding, sessionId });
      }
      controller.abort();
      await close();
      await client.close();
      let refusal: unknown;
      try {
        await client.plugins.readTrajectory({ ...binding, sessionId });
      } catch (error) {
        refusal = error;
      }
      expect(refusal instanceof RpcTransportError, "retired client accepted a plugin read");
      expect(refusal.message === "client closed", "retired client returned a different failure");
    }
  } catch (error) {
    failures.push(error);
  }
  for (const pending of [
    () => pages.map((close) => close()),
    () => clients.map((client) => client.close()),
  ]) {
    const results = await Promise.allSettled(pending());
    failures.push(
      ...results.flatMap((result) => (result.status === "rejected" ? [result.reason] : [])),
    );
  }
  if (failures.length === 1) throw failures[0];
  if (failures.length) throw new AggregateError(failures, "native gate failed and cleanup failed");
}

void run().then(
  () => Call.ByName("main.NativeRuntimeGate.Complete", ""),
  (error: unknown) =>
    Call.ByName(
      "main.NativeRuntimeGate.Complete",
      error instanceof Error ? error.message : "native gate failed",
    ),
);
