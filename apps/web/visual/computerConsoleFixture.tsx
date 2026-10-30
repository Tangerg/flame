import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import type { PluginComponent, ComponentContext } from "@flame/component-sdk";
import * as stylex from "@stylexjs/stylex";
import { ScrollArea } from "@/ui/index";
import { AgentWorkspaceView } from "@/ui/agent/index";
import { createFlameClient, createHttpTransport } from "@flame/runtime-client";
import { stringifyJSON } from "@qimu/sdk/json";
import type { CallPluginRequest, ServerCapabilities } from "@flame/runtime-client/wire";
import { CAPABILITY_JSON } from "../../../plugins/computer/component/methods.generated";
import type { OperationRecord, Snapshot, Surface } from "../../../plugins/computer/component/types";

const modules = import.meta.glob("../../../plugins/computer/package/**/components/component.js");
const installationId = "55667788-1234-4234-9234-123456789abc";
const digest = "c".repeat(64);
const host = { hostname: "isolated-desktop.example", platform: "linux", arch: "x64" };
const capturedImage =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==";

const capturedAudio =
  "UklGRkQAAABXQVZFZm10IBAAAAABAAEAQB8AAIA+AAACABAAZGF0YSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==";

const styles = stylex.create({
  frame: { display: "flex", height: "100dvh", minHeight: 0, minWidth: 0 },
});

function frame(value: unknown) {
  return `event: message\ndata: ${JSON.stringify(value)}\n\n`;
}

export async function createComputerConsoleFixture(locale: string) {
  const load = Object.values(modules);
  if (load.length !== 1) throw new Error("computer fixture requires one built component");
  const published = (await load[0]!()) as { Computer: PluginComponent };
  if (typeof published.Computer !== "function")
    throw new Error("computer component export is missing");
  const Computer = published.Computer;
  const owner = new AbortController();
  const subscribers = new Set<ReadableStreamDefaultController<Uint8Array>>();
  const encoder = new TextEncoder();
  let snapshot: Snapshot = { host, configured: true };
  const records = new Map<string, OperationRecord>();
  const media = new URLSearchParams(location.search).get("media");
  if (media === "unreadable") {
    for (const [id, data] of [
      ["recorded-unreadable-image", "aW52YWxpZCBQTkc="],
      ["recorded-valid-image", capturedImage],
    ] as const) {
      records.set(id, {
        id,
        kind: "tool",
        method: "computer_js",
        admittedAt: "2026-10-29T00:00:00.000Z",
        dispatched: true,
        settled: true,
        inputJSON: '{"code":"recorded capture"}',
        invocationJSON: '{"origin":"model"}',
        outcomeJSON: JSON.stringify({
          content: [{ type: "image", mimeType: "image/png", data }],
        }),
      });
    }
  } else if (media === "links") {
    records.set("recorded-resource-links", {
      id: "recorded-resource-links",
      kind: "tool",
      method: "computer_js",
      admittedAt: "2026-10-29T00:00:00.000Z",
      dispatched: true,
      settled: true,
      inputJSON: '{"code":"recorded resource links"}',
      invocationJSON: '{"origin":"model"}',
      outcomeJSON: JSON.stringify({
        content: [
          {
            type: "resource_link",
            title: "",
            name: "Workspace report — 原始说明",
            uri: "https://example.invalid/review/captured-workspace-report-with-retained-source.md",
          },
          {
            type: "resource_link",
            title: " \t ",
            name: " ",
            uri: "https://example.invalid/review/another-captured-file-with-original-identity-and-no-display-title.md",
          },
        ],
      }),
    });
  } else if (media === "unreadable-audio") {
    records.set("recorded-unreadable-audio", {
      id: "recorded-unreadable-audio",
      kind: "tool",
      method: "computer_js",
      admittedAt: "2026-10-29T00:00:00.000Z",
      dispatched: true,
      settled: true,
      inputJSON: '{"code":"recorded audio"}',
      invocationJSON: '{"origin":"model"}',
      outcomeJSON: JSON.stringify({
        content: [
          {
            type: "resource",
            resource: {
              uri: "memory://captures/2026-10-29/review/unreadable-audio-with-retained-source-identity.wav",
              mimeType: "audio/wav",
              blob: "aW52YWxpZCBXQVY=",
            },
          },
          { type: "audio", mimeType: "audio/wav", data: capturedAudio },
        ],
      }),
    });
  } else if (media === "resources") {
    records.set("recorded-resources", {
      id: "recorded-resources",
      kind: "tool",
      method: "computer_js",
      admittedAt: "2026-10-29T00:00:00.000Z",
      dispatched: true,
      settled: true,
      inputJSON: '{"code":"recorded resources"}',
      invocationJSON: '{"origin":"model"}',
      outcomeJSON: JSON.stringify({
        content: [
          {
            type: "resource",
            resource: {
              uri: "memory://captures/2026-10-29/review/desktop-window-with-source-context-and-retained-original-identity.png",
              mimeType: "image/png",
              blob: capturedImage,
            },
          },
          {
            type: "resource",
            resource: {
              uri: "memory://captures/2026-10-29/review/audio-stream-with-source-context-and-retained-original-identity.wav",
              mimeType: "audio/wav",
              blob: capturedAudio,
            },
          },
          {
            type: "resource",
            resource: {
              uri: "memory://captures/2026-10-29/review/notes.txt",
              mimeType: "text/plain",
              text: "Captured source remains literal.\n",
            },
          },
        ],
      }),
    });
  }
  const commands = new Map<string, CallPluginRequest>();
  const outcomes = new Map<string, unknown>();
  let pending:
    { key: string; settle(response: Response): void; requestId: string | number } | undefined;

  function publish(next: Snapshot) {
    snapshot = next;
    const notification = frame({
      jsonrpc: "2.0",
      method: "notifications.plugin.stream",
      params: { kind: "chunk", value: stringifyJSON(snapshot) },
    });
    for (const subscriber of subscribers) subscriber.enqueue(encoder.encode(notification));
  }
  function surface(): Surface {
    const session = snapshot.session;
    if (!session) throw new Error("computer fixture has no session");
    return {
      host,
      sessionId: session.id,
      workerId: session.workerId,
      surfaceDigest: session.surfaceDigest!,
      expiresAt: session.expiresAt,
      instructions:
        "This isolated graphical fixture does not control a real desktop. Submit code to inspect its permission and result presentation.",
      toolsJSON:
        '{"tools":[{"name":"js","inputSchema":{"type":"object","properties":{"code":{"type":"string"}},"required":["code"]}},{"name":"js_reset","inputSchema":{"type":"object","properties":{}}}]}',
      serverJSON: '{"name":"computer-browser-fixture","version":"1"}',
    };
  }
  const transport = createHttpTransport({
    baseUrl: "https://computer-fixture.invalid",
    fetch: async (_url, init) => {
      const request = JSON.parse(init!.body as string) as {
        id: string | number;
        method: string;
        params: CallPluginRequest;
      };
      const params = request.params;
      const key = new Headers(init!.headers).get("Idempotency-Key");
      const success = (result: unknown) =>
        Response.json({ jsonrpc: "2.0", id: request.id, result });
      const output = (value: unknown) => success({ output: stringifyJSON(value) });
      if (request.method === "plugins.getCommand") {
        return success(commands.get((params as unknown as { key: string }).key));
      }
      if (request.method === "plugins.stream") {
        let stream: ReadableStreamDefaultController<Uint8Array>;
        const remove = () => {
          if (subscribers.delete(stream)) stream.close();
        };
        return new Response(
          new ReadableStream<Uint8Array>({
            start(controller) {
              stream = controller;
              subscribers.add(controller);
              controller.enqueue(
                encoder.encode(frame({ jsonrpc: "2.0", id: request.id, result: {} })),
              );
              publish(snapshot);
              init?.signal?.addEventListener("abort", remove, { once: true });
            },
            cancel() {
              subscribers.delete(stream);
              init?.signal?.removeEventListener("abort", remove);
            },
          }),
          { headers: { "Content-Type": "text/event-stream" } },
        );
      }
      if (request.method !== "plugins.call")
        throw new Error("unsupported computer fixture request");
      const method = params.method;
      const input = JSON.parse(params.input) as Record<string, unknown>;
      if (method === "flame.computer/read.v1") return output(snapshot);
      if (method === "flame.computer/context.v1") return output(surface());
      if (method === "flame.computer/history.v1")
        return output({
          entries: [...records.values()].map(
            ({ inputJSON: _input, invocationJSON: _invocation, outcomeJSON: _outcome, ...entry }) =>
              entry,
          ),
        });
      if (method === "flame.computer/result.v1") return output(records.get(input.id as string));
      if (!key) throw new Error("fixture command omitted its identity");
      if (commands.has(key)) {
        if (outcomes.has(key)) return output(outcomes.get(key));
        return Response.json({
          jsonrpc: "2.0",
          id: request.id,
          error: {
            code: -32603,
            message: "the original result is unconfirmed",
            data: { type: "internal_error" },
          },
        });
      }
      commands.set(key, params);
      let result: unknown;
      if (method === "flame.computer/open.v1") {
        publish({
          host,
          configured: true,
          session: {
            id: "control-browser-fixture",
            workerId: "worker-browser-fixture",
            state: "ready",
            surfaceDigest: digest,
            openedAt: new Date(Date.now()).toISOString(),
            expiresAt: new Date(Date.now() + Number(input.leaseSeconds) * 1000).toISOString(),
          },
        });
        result = snapshot.session;
      } else if (method === "flame.computer/execute.v1") {
        records.set(key, {
          id: key,
          kind: "command",
          method,
          inputJSON: params.input,
          invocationJSON: '{"origin":"human"}',
          admittedAt: new Date(Date.now()).toISOString(),
          dispatched: true,
          settled: false,
        });
        publish({
          ...snapshot,
          session: {
            ...snapshot.session!,
            state: "executing",
            operation: { id: key, tool: "js", startedAt: new Date(Date.now()).toISOString() },
            approval: {
              id: "permission-browser-fixture",
              operationId: key,
              sessionId: snapshot.session!.id,
              requestJSON: JSON.stringify({
                mode: "form",
                message: "Allow control of the isolated example editor?",
                requestedSchema: { type: "object", properties: {} },
                _meta: {
                  codex_approval_kind: "mcp_tool_call",
                  connector_id: "computer-use",
                  tool_params: { app: "com.example.isolated-editor" },
                  persist: ["session"],
                  riskLevel: "high",
                  warningSubtitle: "The editor can write files on the isolated Runtime host.",
                },
              }),
            },
          },
        });
        return await new Promise<Response>((resolve, reject) => {
          pending = { key, settle: resolve, requestId: request.id };
          init?.signal?.addEventListener("abort", () => reject(init.signal!.reason), {
            once: true,
          });
        });
      } else if (method === "flame.computer/reply.v1") {
        if (!pending || input.requestId !== snapshot.session?.approval?.id)
          return success({
            failure: {
              code: "permission_retired",
              message: "the original permission request ended",
            },
          });
        const original = pending;
        pending = undefined;
        const outcome = {
          commandId: original.key,
          result: {
            content: [
              { type: "text", text: "The original isolated fixture operation completed." },
              { type: "image", mimeType: "image/png", data: capturedImage },
            ],
          },
          cleanup: "ended",
        };
        outcomes.set(original.key, outcome);
        records.set(original.key, {
          ...records.get(original.key)!,
          settled: true,
          outcomeJSON: JSON.stringify({ value: outcome }),
        });
        publish({
          ...snapshot,
          session: {
            ...snapshot.session!,
            state: "ready",
            operation: undefined,
            approval: undefined,
          },
        });
        original.settle(
          Response.json({
            jsonrpc: "2.0",
            id: original.requestId,
            result: { output: stringifyJSON(outcome) },
          }),
        );
        result = true;
      } else if (method === "flame.computer/close.v1" || method === "flame.computer/stop.v1") {
        publish({
          ...snapshot,
          session: {
            ...snapshot.session!,
            state: "closed",
            reason: "user",
            operation: undefined,
            approval: undefined,
          },
        });
        if (pending) {
          pending.settle(
            Response.json({
              jsonrpc: "2.0",
              id: pending.requestId,
              error: {
                code: -32603,
                message: "original execution outcome unconfirmed",
                data: { type: "internal_error" },
              },
            }),
          );
          pending = undefined;
        }
        result = snapshot.session;
      } else throw new Error("unsupported computer fixture method");
      outcomes.set(key, result);
      return output(result);
    },
  });
  const journal = new Map<string, unknown>();
  const client = createFlameClient(transport, {
    mutationStorage: {
      get: (key) => structuredClone(journal.get(key)),
      set: (key, value) => {
        journal.set(key, structuredClone(value));
      },
      remove: (key) => {
        journal.delete(key);
      },
      keys: () => [...journal.keys()],
    },
    capabilities: () =>
      ({
        features: { plugins: { enabled: true } },
        limits: { idempotency: { namespace: "idp_" + "4".repeat(32), retentionSeconds: 3600 } },
      }) as unknown as ServerCapabilities,
  });
  const context: ComponentContext = {
    component: { installationId, digest, componentId: "computer" },
    binding: {
      installationId,
      digest,
      contribution: "computer-control",
      contract: CAPABILITY_JSON,
    },
    signal: owner.signal,
    runtime: client,
    locale,
    telemetry: { read: () => ({ spans: [], logs: [], metrics: [] }), clear: () => undefined },
    track: (operation) => operation(owner.signal),
    saveFile: async () => "requested",
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose: () => {
      throw new Error("computer fixture has no composition");
    },
  };
  return {
    node: (
      <div {...stylex.props(styles.frame)}>
        <AgentWorkspaceView>
          <ScrollArea>
            <Computer context={context} />
          </ScrollArea>
        </AgentWorkspaceView>
      </div>
    ),
    client,
    close: async () => {
      owner.abort();
      await client.close();
    },
  };
}
