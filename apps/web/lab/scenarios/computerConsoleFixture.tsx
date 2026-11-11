import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import type { PluginComponent, ComponentContext } from "@flame/component-sdk";
import * as stylex from "@stylexjs/stylex";
import { ScrollArea } from "@/ui/index";
import { AgentWorkspaceView } from "@/ui/agent/index";
import { createFlameClient, createHttpTransport } from "@flame/runtime-client";
import { canonicalJSON, parseJSON, stringifyJSON } from "@frame/sdk/json";
import type { CallPluginRequest, ServerCapabilities } from "@flame/runtime-client/wire";
import { CAPABILITY_JSON } from "../../../../plugins/computer/component/methods.generated";
import type {
  HistoryPage,
  OperationRecord,
  Snapshot,
  Surface,
} from "../../../../plugins/computer/component/types";
import { LIMITS } from "../../../../plugins/computer/contracts/limits";
import {
  receiptCapacity,
  type ReceiptEvidence,
} from "../../../../plugins/computer/backend/receiptEvidence";
import { COMPUTER_FIXTURE_VARIANTS } from "./computerFixtureStates";
import { COMPUTER_IMAGE_SAMPLES, createComputerFixtureImage } from "./computerFixtureImages";

const modules = import.meta.glob("../../../../plugins/computer/package/**/components/component.js");
const installationId = "55667788-1234-4234-9234-123456789abc";
const digest = "c".repeat(64);
const host = { hostname: "isolated-desktop.example", platform: "linux", arch: "x64" };
const capturedAudio =
  "UklGRkQAAABXQVZFZm10IBAAAAABAAEAQB8AAIA+AAACABAAZGF0YSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==";
const encoder = new TextEncoder();

const styles = stylex.create({
  frame: { display: "flex", height: "100dvh", minHeight: 0, minWidth: 0 },
});

type Receipt = ReceiptEvidence & {
  readonly kind: "tool" | "command";
  dispatched: boolean;
  outcome?: string;
};

function frame(value: unknown) {
  return `event: message\ndata: ${JSON.stringify(value)}\n\n`;
}

async function digestJSON(value: unknown) {
  const hash = await crypto.subtle.digest("SHA-256", encoder.encode(canonicalJSON(value)));
  return [...new Uint8Array(hash)].map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

async function original(
  id: string,
  kind: Receipt["kind"],
  method: string,
  input: unknown,
  origin: "human" | "model",
  target: CallPluginRequest["target"],
  admittedAt = new Date(Date.now()).toISOString(),
): Promise<Receipt> {
  return {
    id,
    kind,
    method,
    input: canonicalJSON(input),
    inputDigest: await digestJSON(input),
    invocation: canonicalJSON({ id, origin, target }),
    admittedAt,
    dispatched: false,
  };
}

async function recorded(id: string, code: string, result: unknown): Promise<Receipt> {
  return {
    ...(await original(
      id,
      "tool",
      "js",
      { code },
      "model",
      { scope: "installation", reference: installationId },
      "2026-10-29T00:00:00.000Z",
    )),
    dispatched: true,
    outcome: canonicalJSON(result),
  };
}

function record(receipt: Receipt): OperationRecord {
  return {
    id: receipt.id,
    kind: receipt.kind,
    method: receipt.method,
    admittedAt: receipt.admittedAt,
    dispatched: receipt.dispatched,
    settled: receipt.outcome !== undefined,
    inputJSON: receipt.input,
    invocationJSON: receipt.invocation,
    ...(receipt.outcome !== undefined ? { outcomeJSON: receipt.outcome } : {}),
  };
}

function replay(receipt: Receipt, requestId: string | number): Response {
  if (receipt.outcome === undefined)
    return Response.json({
      jsonrpc: "2.0",
      id: requestId,
      error: {
        code: -32603,
        message: "internal_error",
        data: { type: "internal_error", detail: "the original result is unconfirmed" },
      },
    });
  const reply = parseJSON(receipt.outcome) as { value: unknown } | { failure: unknown };
  return Response.json({
    jsonrpc: "2.0",
    id: requestId,
    result: "value" in reply ? { output: stringifyJSON(reply.value) } : reply,
  });
}

export async function createComputerConsoleFixture(locale: string, requestedState: string | null) {
  const state = COMPUTER_FIXTURE_VARIANTS.find(({ id }) => id === requestedState)?.id ?? "default";
  const load = Object.values(modules);
  if (load.length !== 1) throw new Error("computer fixture requires one built component");
  const published = (await load[0]!()) as { Computer: PluginComponent };
  if (typeof published.Computer !== "function")
    throw new Error("computer component export is missing");
  const Computer = published.Computer;
  const owner = new AbortController();
  const subscribers = new Set<ReadableStreamDefaultController<Uint8Array>>();
  let current: Omit<Snapshot, "receipts"> = { host, configured: true };
  const seeded: Promise<Receipt>[] = [];
  const capturedImage = createComputerFixtureImage(COMPUTER_IMAGE_SAMPLES[0]);
  if (state === "images") {
    for (const sample of COMPUTER_IMAGE_SAMPLES) {
      seeded.push(
        recorded(sample.id, "recorded capture", {
          content: [
            { type: "image", mimeType: "image/png", data: createComputerFixtureImage(sample) },
          ],
        }),
      );
    }
  } else if (state === "unreadable") {
    for (const [id, data] of [
      ["recorded-unreadable-image", "aW52YWxpZCBQTkc="],
      ["recorded-valid-image", capturedImage],
    ] as const) {
      seeded.push(
        recorded(id, "recorded capture", {
          content: [{ type: "image", mimeType: "image/png", data }],
        }),
      );
    }
  } else if (state === "links") {
    seeded.push(
      recorded("recorded-resource-links", "recorded resource links", {
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
    );
  } else if (state === "unreadable-audio") {
    seeded.push(
      recorded("recorded-unreadable-audio", "recorded audio", {
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
    );
  } else if (state === "resources") {
    seeded.push(
      recorded("recorded-resources", "recorded resources", {
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
    );
  }
  // Retained original evidence is the only receipt owner; capacity, history and
  // results are projected from it.
  const evidence = new Map(
    (await Promise.all(seeded)).map((receipt) => [receipt.id, receipt] as const),
  );
  const commands = new Map<string, CallPluginRequest>();
  let pending:
    { key: string; settle(response: Response): void; requestId: string | number } | undefined;

  function snapshot(): Snapshot {
    return { ...current, receipts: receiptCapacity(evidence.values(), LIMITS) };
  }
  function settle(id: string, outcome: unknown) {
    evidence.get(id)!.outcome = canonicalJSON(outcome);
  }
  function history(input: Record<string, unknown>): HistoryPage {
    const receipts = [...evidence.values()].reverse();
    const start = input.before ? receipts.findIndex(({ id }) => id === input.before) + 1 : 0;
    if (start === 0 && input.before) throw new Error("computer fixture cursor is unknown");
    const page = receipts.slice(start, start + Number(input.limit));
    return {
      entries: page.map((receipt) => {
        const {
          inputJSON: _input,
          invocationJSON: _invocation,
          outcomeJSON: _outcome,
          ...entry
        } = record(receipt);
        return entry;
      }),
      ...(start + page.length < receipts.length ? { next: page.at(-1)!.id } : {}),
    };
  }
  function publish(next: Omit<Snapshot, "receipts">) {
    current = next;
    const notification = frame({
      jsonrpc: "2.0",
      method: "notifications.plugin.stream",
      params: { kind: "chunk", value: stringifyJSON(snapshot()) },
    });
    for (const subscriber of subscribers) subscriber.enqueue(encoder.encode(notification));
  }
  function surface(): Surface {
    const session = current.session;
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
              publish(current);
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
      if (method === "flame.computer/read.v1") return output(snapshot());
      if (method === "flame.computer/context.v1") return output(surface());
      if (method === "flame.computer/history.v1") return output(history(input));
      if (method === "flame.computer/result.v1") {
        const receipt = evidence.get(input.id as string);
        return output(receipt && record(receipt));
      }
      if (!key) throw new Error("fixture command omitted its identity");
      const admitted = await original(
        key,
        "command",
        method,
        parseJSON(params.input),
        "human",
        params.target,
      );
      const previous = evidence.get(key);
      if (previous) return replay(previous, request.id);
      evidence.set(key, admitted);
      commands.set(key, params);
      let result: unknown;
      if (method === "flame.computer/open.v1") {
        admitted.dispatched = true;
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
        result = current.session;
      } else if (method === "flame.computer/execute.v1") {
        admitted.dispatched = true;
        publish({
          ...current,
          session: {
            ...current.session!,
            state: "executing",
            operation: { id: key, tool: "js", startedAt: new Date(Date.now()).toISOString() },
            approval: {
              id: "permission-browser-fixture",
              operationId: key,
              sessionId: current.session!.id,
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
        if (!pending || input.requestId !== current.session?.approval?.id) {
          const refusal = {
            failure: {
              code: "permission_retired",
              message: "the original permission request ended",
            },
          };
          settle(key, refusal);
          publish(current);
          return success(refusal);
        }
        admitted.dispatched = true;
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
        settle(original.key, { value: outcome });
        const { operation: _operation, approval: _approval, ...session } = current.session!;
        publish({ ...current, session: { ...session, state: "ready" } });
        original.settle(
          Response.json({
            jsonrpc: "2.0",
            id: original.requestId,
            result: { output: stringifyJSON(outcome) },
          }),
        );
        result = true;
      } else if (method === "flame.computer/close.v1" || method === "flame.computer/stop.v1") {
        admitted.dispatched = true;
        const { operation: _operation, approval: _approval, ...session } = current.session!;
        publish({ ...current, session: { ...session, state: "closed", reason: "user" } });
        // An interrupted execution keeps its pending original evidence.
        if (pending) {
          pending.settle(
            Response.json({
              jsonrpc: "2.0",
              id: pending.requestId,
              error: {
                code: -32603,
                message: "internal_error",
                data: { type: "internal_error", detail: "original execution outcome unconfirmed" },
              },
            }),
          );
          pending = undefined;
        }
        result = current.session;
      } else throw new Error("unsupported computer fixture method");
      settle(key, { value: result });
      publish(current);
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
