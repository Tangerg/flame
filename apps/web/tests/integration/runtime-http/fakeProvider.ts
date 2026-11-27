import {
  createServer as createHttpServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

export const liveMCPToolName = "http-e2e-stdio_ping";

export const killedMCPToolName = "http-e2e-kill_ping";

export const compactionCutpointMarker = "E2E_COMPACTION_CUTPOINT";

export const compactionSummaryMarker = "E2E_COMPACTION_SUMMARY";

export const compactionRecoveryMarker = "E2E_COMPACTION_RECOVERY";

export const unavailableProviderStatus = 503;

const compactionContextBytes = 256 * 1024;

const compactionContextUnit = "context ";

// Large results exercise the token boundary without timing a long chain of Tool round trips.
const compactionContextPressure = compactionContextUnit.repeat(
  compactionContextBytes / compactionContextUnit.length,
);

interface FakeChatRequest {
  input?: string | string[];
  messages?: Array<{ role?: string; content?: unknown }>;
  model?: string;
  reasoning_effort?: string;
  stream?: boolean;
  tools?: Array<{ function?: { name?: string } }>;
}

interface FakeToolCall {
  arguments: string;
  name: string;
}

interface Deferred {
  promise: Promise<void>;
  resolve: () => void;
}

interface ProviderGateOptions {
  failureStatus?: number;
  minimumToolResults?: number;
  requiredTool?: string;
}

export interface ProviderGate extends ProviderGateOptions {
  arrived: Deferred;
  closed: Deferred;
  marker: string;
  minimumToolResults: number;
  request?: FakeChatRequest;
  release: Deferred;
}

function deferred(): Deferred {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

export function createProviderGate(
  marker: string,
  options: ProviderGateOptions = {},
): ProviderGate {
  return {
    ...options,
    arrived: deferred(),
    closed: deferred(),
    marker,
    minimumToolResults: options.minimumToolResults ?? 0,
    release: deferred(),
  };
}

async function requestJson(request: IncomingMessage): Promise<FakeChatRequest> {
  const chunks: Buffer[] = [];
  for await (const chunk of request) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8")) as FakeChatRequest;
}

type ScriptedReply = { text?: string; tool?: FakeToolCall; tools?: FakeToolCall[] };

// One provider request as the script sees it: transcript markers select a
// scenario, offered tools gate a step, and tool results advance it.
interface ScriptTurn {
  readonly transcript: string;
  readonly tools: ReadonlySet<string>;
  readonly toolResults: number;
}

interface ScriptRule {
  when(turn: ScriptTurn): boolean;
  reply(turn: ScriptTurn): ScriptedReply;
}

const said = (turn: ScriptTurn, marker: string) => turn.transcript.includes(marker);

const call = (name: string, input: unknown): ScriptedReply => ({
  tool: { name, arguments: JSON.stringify(input) },
});

// The first matching rule answers; order encodes precedence between scenarios
// whose markers can appear in the same transcript.
const SCRIPT: readonly ScriptRule[] = [
  {
    when: (turn) => said(turn, "E2E_RESUME_INPUT"),
    reply: () => ({ text: "Resume follow-up input observed." }),
  },
  {
    when: (turn) => said(turn, compactionRecoveryMarker),
    reply: () => ({
      text: "Committed compaction recovered without an immediate second compaction.",
    }),
  },
  {
    when: (turn) => said(turn, compactionSummaryMarker) && turn.tools.has("get_goal"),
    reply: () => ({ text: "The post-compaction model call completed." }),
  },
  {
    when: (turn) => said(turn, compactionCutpointMarker) && turn.tools.has("set_plan"),
    reply: (turn) => ({
      text: compactionContextPressure,
      ...call("set_plan", {
        steps: [
          { description: `Process context batch ${turn.toolResults + 1}`, status: "in_progress" },
        ],
      }),
    }),
  },
  {
    when: (turn) =>
      turn.tools.size === 0 &&
      (said(turn, compactionCutpointMarker) || said(turn, compactionSummaryMarker)),
    reply: () => ({
      text: `${compactionSummaryMarker}: preserve only the durable progress boundary.`,
    }),
  },
  {
    when: (turn) =>
      turn.tools.has("report_goal_outcome") &&
      (((said(turn, "E2E_GOAL_SETTLEMENT") || said(turn, "E2E_GOAL_STOP")) &&
        turn.toolResults === 0) ||
        (said(turn, "E2E_GOAL_AFTER_INPUT") && turn.toolResults === 1)),
    reply: () => call("report_goal_outcome", { outcome: "completed" }),
  },
  {
    when: (turn) => said(turn, "E2E_HITL") && turn.tools.has("ask_user") && turn.toolResults === 0,
    reply: () =>
      call("ask_user", {
        questions: [{ question: "Continue the HTTP lifecycle check?", header: "Continue" }],
      }),
  },
  {
    when: (turn) =>
      said(turn, "E2E_PLAN_REJECTED") && turn.tools.has("set_plan") && turn.toolResults === 1,
    reply: () =>
      call("set_plan", {
        steps: [
          { description: "Start the first task", status: "in_progress" },
          { description: "Start the second task", status: "in_progress" },
        ],
      }),
  },
  {
    when: (turn) => said(turn, "E2E_PLAN") && turn.tools.has("set_plan") && turn.toolResults === 0,
    reply: () =>
      call("set_plan", {
        steps: [
          { description: "Inspect the runtime contract", status: "completed" },
          { description: "Verify frontend reconciliation", status: "in_progress" },
        ],
      }),
  },
  {
    when: (turn) =>
      said(turn, "E2E_PARALLEL_EDITED_TOOL") && turn.tools.has("shell") && turn.toolResults === 0,
    reply: () => ({
      tools: [
        {
          name: "shell",
          arguments: JSON.stringify({
            command: "printf approval-original",
            description: "Print the original approval marker",
          }),
        },
        {
          name: "shell",
          arguments: JSON.stringify({
            command: "printf approval-sibling",
            description: "Print the sibling approval marker",
          }),
        },
      ],
    }),
  },
  {
    when: (turn) => said(turn, "E2E_APPROVAL") && turn.tools.has("shell") && turn.toolResults === 0,
    reply: () =>
      call("shell", { command: "printf approval-ok", description: "Print the approval marker" }),
  },
  {
    when: (turn) =>
      said(turn, "E2E_FILE_WRITE") &&
      turn.tools.has("read") &&
      turn.tools.has("apply_patch") &&
      turn.toolResults === 0,
    reply: () => call("read", { path: "agent-write.txt" }),
  },
  {
    when: (turn) =>
      said(turn, "E2E_FILE_WRITE") && turn.tools.has("apply_patch") && turn.toolResults === 1,
    reply: () =>
      call("apply_patch", {
        patch: [
          "--- a/agent-write.txt",
          "+++ b/agent-write.txt",
          "@@ -1 +1 @@",
          "-before",
          "+after",
          "",
        ].join("\n"),
      }),
  },
  {
    when: (turn) =>
      (said(turn, "E2E_SKILL_PROPOSAL_PROJECT") || said(turn, "E2E_SKILL_PROPOSAL_USER")) &&
      turn.tools.has("search_tools") &&
      !turn.tools.has("propose_skill") &&
      turn.toolResults === 0,
    reply: () => call("search_tools", { query: "select:propose_skill" }),
  },
  {
    when: (turn) =>
      said(turn, "E2E_SKILL_PROPOSAL_PROJECT") &&
      turn.tools.has("propose_skill") &&
      turn.toolResults === 1,
    reply: () =>
      call("propose_skill", {
        name: "e2e-project-proposal",
        description: "Preserve the project-side HTTP E2E workflow.",
        instructions: "Run the project-side HTTP E2E workflow and report its result.",
        scope: "repository",
      }),
  },
  {
    when: (turn) =>
      said(turn, "E2E_SKILL_PROPOSAL_USER") &&
      turn.tools.has("propose_skill") &&
      turn.toolResults === 1,
    reply: () =>
      call("propose_skill", {
        name: "e2e-user-proposal",
        description: "Preserve the user-side HTTP E2E workflow.",
        instructions: "Run the user-side HTTP E2E workflow and report its result.",
        scope: "user",
      }),
  },
  {
    when: (turn) =>
      said(turn, "E2E_MCP_TOOL") &&
      turn.tools.has("search_tools") &&
      !turn.tools.has(liveMCPToolName) &&
      turn.toolResults === 0,
    reply: () => call("search_tools", { query: `select:${liveMCPToolName}` }),
  },
  {
    when: (turn) =>
      said(turn, "E2E_MCP_TOOL") && turn.tools.has(liveMCPToolName) && turn.toolResults === 1,
    reply: () => call(liveMCPToolName, {}),
  },
  {
    when: (turn) => said(turn, "E2E_MCP_TOOL") && turn.toolResults >= 2,
    reply: (turn) => ({ text: said(turn, "pong") ? "MCP pong observed." : "MCP pong missing." }),
  },
  {
    when: (turn) =>
      said(turn, "E2E_FORCE_KILL_TOOL") &&
      turn.tools.has("search_tools") &&
      !turn.tools.has(killedMCPToolName) &&
      turn.toolResults === 0,
    reply: () => call("search_tools", { query: `select:${killedMCPToolName}` }),
  },
  {
    when: (turn) => said(turn, "E2E_FORCE_KILL_TOOL") && turn.tools.has(killedMCPToolName),
    reply: () => call(killedMCPToolName, {}),
  },
];

function scriptedReply(body: FakeChatRequest): ScriptedReply {
  const turn: ScriptTurn = {
    transcript: JSON.stringify(body.messages ?? []),
    tools: new Set(
      (body.tools ?? []).flatMap((tool) => (tool.function?.name ? [tool.function.name] : [])),
    ),
    toolResults: (body.messages ?? []).filter((message) => message.role === "tool").length,
  };
  return (
    SCRIPT.find((rule) => rule.when(turn))?.reply(turn) ?? {
      text: "HTTP runtime lifecycle complete.",
    }
  );
}

function fakeEmbedding(text: string): number[] {
  const normalized = text.toLowerCase();
  if (normalized.includes("semantic target marker")) return [1, 0, 0];
  if (normalized.includes("unrelated helper")) return [0, 1, 0];
  return [0, 0, 1];
}

function writeChatCompletion(
  response: ServerResponse,
  body: FakeChatRequest,
  sequence: number,
): void {
  const id = `chatcmpl-e2e-${sequence}`;
  const model = body.model ?? "e2e-model";
  const reply = scriptedReply(body);
  const tools = reply.tools ?? (reply.tool ? [reply.tool] : []);
  const finishReason = tools.length > 0 ? "tool_calls" : "stop";
  const callID = (index: number) =>
    index === 0 ? `call-e2e-${sequence}` : `call-e2e-${sequence}-${index}`;
  const message =
    tools.length > 0
      ? {
          role: "assistant",
          content: reply.text ?? "",
          tool_calls: tools.map((tool, index) => ({
            id: callID(index),
            type: "function",
            function: { name: tool.name, arguments: tool.arguments },
          })),
        }
      : { role: "assistant", content: reply.text };
  const usage = JSON.stringify(body.messages ?? []).includes("E2E_COMPACTION_")
    ? undefined
    : { prompt_tokens: 8, completion_tokens: 4, total_tokens: 12 };

  if (!body.stream) {
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end(
      JSON.stringify({
        id,
        object: "chat.completion",
        model,
        choices: [{ index: 0, message, finish_reason: finishReason }],
        usage,
      }),
    );
    return;
  }

  response.writeHead(200, { "Content-Type": "text/event-stream" });
  const chunks =
    tools.length > 0
      ? [
          {
            choices: [
              { index: 0, delta: { role: "assistant", content: reply.text }, finish_reason: null },
            ],
          },
          {
            choices: [
              {
                index: 0,
                delta: {
                  tool_calls: tools.map((tool, index) => ({
                    index,
                    id: callID(index),
                    type: "function",
                    function: { name: tool.name, arguments: tool.arguments },
                  })),
                },
                finish_reason: null,
              },
            ],
          },
          { choices: [{ index: 0, delta: {}, finish_reason: finishReason }] },
        ]
      : [
          {
            choices: [
              {
                index: 0,
                delta: { role: "assistant", content: reply.text },
                finish_reason: finishReason,
              },
            ],
          },
        ];
  for (const chunk of chunks) {
    response.write(
      `data: ${JSON.stringify({ id, object: "chat.completion.chunk", model, ...chunk })}\n\n`,
    );
  }
  response.write(
    `data: ${JSON.stringify({
      id,
      object: "chat.completion.chunk",
      model,
      choices: [],
      usage,
    })}\n\n`,
  );
  response.end("data: [DONE]\n\n");
}

export interface FakeProvider {
  readonly baseUrl: string;
  close(): Promise<void>;
}

// Each request reads the current gate, so a test can install or clear one
// while the Runtime keeps its provider connection.
function writeModels(response: ServerResponse) {
  response.writeHead(200, { "Content-Type": "application/json" });
  response.end(JSON.stringify({ object: "list", data: [{ id: "e2e-model", object: "model" }] }));
}

async function writeEmbeddings(request: IncomingMessage, response: ServerResponse) {
  const body = await requestJson(request);
  const inputs = Array.isArray(body.input) ? body.input : [body.input ?? ""];
  response.writeHead(200, { "Content-Type": "application/json" });
  response.end(
    JSON.stringify({
      object: "list",
      model: body.model ?? "e2e-embedding",
      data: inputs.map((input, index) => ({
        object: "embedding",
        index,
        embedding: fakeEmbedding(input),
      })),
      usage: { prompt_tokens: inputs.length, total_tokens: inputs.length },
    }),
  );
}

function writeInjectedFailure(response: ServerResponse, status: number) {
  response.writeHead(status, { "Content-Type": "application/json" });
  response.end(JSON.stringify({ error: { message: "injected provider failure" } }));
}

// A gate captures the first request that carries its marker, offers its
// required tool and has enough tool results.
function gateCaptures(gate: ProviderGate | undefined, body: FakeChatRequest): gate is ProviderGate {
  return (
    gate !== undefined &&
    gate.request === undefined &&
    JSON.stringify(body.messages ?? []).includes(gate.marker) &&
    (gate.requiredTool === undefined ||
      (body.tools ?? []).some((tool) => tool.function?.name === gate.requiredTool)) &&
    (body.messages ?? []).filter((message) => message.role === "tool").length >=
      gate.minimumToolResults
  );
}

// Holds the captured request until the test releases it; reports whether the
// request was settled there, by its client leaving or by an injected failure.
async function holdAtGate(gate: ProviderGate, body: FakeChatRequest, response: ServerResponse) {
  gate.request = body;
  response.once("close", gate.closed.resolve);
  gate.arrived.resolve();
  await gate.release.promise;
  if (response.destroyed) return true;
  if (gate.failureStatus === undefined) return false;
  writeInjectedFailure(response, gate.failureStatus);
  return true;
}

export async function startFakeProvider(
  currentGate: () => ProviderGate | undefined,
): Promise<FakeProvider> {
  let providerCalls = 0;
  const answerChat = async (request: IncomingMessage, response: ServerResponse) => {
    const body = await requestJson(request);
    const gate = currentGate();
    if (gateCaptures(gate, body) && (await holdAtGate(gate, body, response))) return;
    // Once captured, a failing gate keeps failing every request in its scenario.
    if (
      gate?.request !== undefined &&
      gate.failureStatus !== undefined &&
      JSON.stringify(body.messages ?? []).includes(gate.marker)
    ) {
      writeInjectedFailure(response, gate.failureStatus);
      return;
    }
    writeChatCompletion(response, body, ++providerCalls);
  };
  const provider = createHttpServer(async (request, response) => {
    try {
      if (request.method === "GET" && request.url?.endsWith("/models")) writeModels(response);
      else if (request.method === "POST" && request.url?.endsWith("/embeddings"))
        await writeEmbeddings(request, response);
      else if (request.method === "POST" && request.url?.endsWith("/chat/completions"))
        await answerChat(request, response);
      else response.writeHead(404).end();
    } catch (error) {
      response.writeHead(500, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ error: { message: String(error) } }));
    }
  });
  await new Promise<void>((resolve, reject) => {
    provider.once("error", reject);
    provider.listen(0, "127.0.0.1", resolve);
  });
  const providerAddress = provider.address();
  if (providerAddress === null || typeof providerAddress === "string") {
    throw new Error("failed to start fake model provider");
  }
  return {
    baseUrl: `http://127.0.0.1:${providerAddress.port}`,
    close: async () => {
      if (provider.listening)
        await new Promise<void>((resolve, reject) =>
          provider.close((error) => (error ? reject(error) : resolve())),
        );
    },
  };
}
