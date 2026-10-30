import { METHODS } from "./methods";
import { z } from "zod";
import { PROTOCOL, type CapabilityDescriptor } from "@qimu/node";
import { LIMITS } from "./limits";

const token = z.string().min(1).max(256);
const digest = z.string().regex(/^[a-f0-9]{64}$/);
const timestamp = z.iso.datetime();
const empty = z.strictObject({});
const sessionInput = z.strictObject({ sessionId: token });
const resultJSON = z.string().max(LIMITS.maxResultBytes);
const approval = z.strictObject({
  id: token,
  sessionId: token,
  operationId: token,
  requestJSON: z.string().max(LIMITS.maxApprovalBytes),
});
const operation = z.strictObject({
  id: token,
  tool: z.enum(["js", "js_reset"]),
  startedAt: timestamp,
});
const session = z.strictObject({
  id: token,
  workerId: token,
  state: z.enum([
    "opening",
    "ready",
    "executing",
    "stopping",
    "closed",
    "faulted",
  ]),
  openedAt: timestamp,
  expiresAt: timestamp,
  surfaceDigest: digest.optional(),
  reason: z
    .enum([
      "user",
      "expired",
      "shutdown",
      "engine",
      "cleanup",
      "startup",
      "cancelled",
    ])
    .optional(),
  operation: operation.optional(),
  approval: approval.optional(),
});
const host = z.strictObject({
  hostname: z.string().min(1).max(256),
  platform: z.string().min(1).max(32),
  arch: z.string().min(1).max(32),
});
const snapshot = z.strictObject({
  host,
  configured: z.boolean(),
  session: session.optional(),
});
const surface = z.strictObject({
  sessionId: token,
  workerId: token,
  surfaceDigest: digest,
  expiresAt: timestamp,
  instructions: z.string().min(1).max(LIMITS.maxSurfaceBytes),
  toolsJSON: z.string().max(LIMITS.maxSurfaceBytes),
  serverJSON: z.string().max(LIMITS.maxSurfaceBytes),
  host,
});
const execute = z.strictObject({
  sessionId: token,
  workerId: token,
  surfaceDigest: digest,
  tool: z.enum(["js", "js_reset"]),
  argumentsJSON: z.string().min(2).max(LIMITS.maxInputBytes),
});
const executionResult = z.strictObject({
  commandId: token,
  result: z.record(z.string(), z.unknown()),
  cleanup: z.enum(["ended", "unconfirmed"]),
});
const reply = z.strictObject({
  sessionId: token,
  requestId: token,
  responseJSON: z.string().min(2).max(LIMITS.maxApprovalBytes),
});
const summary = z.strictObject({
  id: token,
  kind: z.enum(["tool", "command"]),
  method: token,
  admittedAt: timestamp,
  dispatched: z.boolean(),
  settled: z.boolean(),
});
const record = summary.extend({
  inputJSON: z.string().max(LIMITS.maxInputBytes),
  invocationJSON: z.string().max(LIMITS.maxInputBytes),
  outcomeJSON: resultJSON.optional(),
});

export const schemas = Object.freeze({
  empty,
  session,
  snapshot,
  surface,
  execute,
  executionResult,
  reply,
  record,
  summary,
  open: z.strictObject({
    leaseSeconds: z
      .number()
      .int()
      .min(LIMITS.minLeaseSeconds)
      .max(LIMITS.maxLeaseSeconds),
  }),
  sessionInput,
  stop: z.strictObject({ sessionId: token, operationId: token }),
  history: z.strictObject({
    before: token.optional(),
    limit: z.number().int().min(1).max(LIMITS.maxHistoryPage),
  }),
  historyPage: z.strictObject({
    entries: z.array(summary).max(LIMITS.maxHistoryPage),
    next: token.optional(),
  }),
  resultInput: z.strictObject({ id: token }),
});

export type Snapshot = z.infer<typeof snapshot>;
export type ControlSession = z.infer<typeof session>;
export type Approval = z.infer<typeof approval>;
export type Surface = z.infer<typeof surface>;
export type ExecuteInput = z.infer<typeof execute>;
export type ExecutionResult = z.infer<typeof executionResult>;
export type ReplyInput = z.infer<typeof reply>;
export type HistoryEntry = z.infer<typeof summary>;
export type HistoryPage = z.infer<typeof schemas.historyPage>;
export type OperationRecord = z.infer<typeof record>;

function schema(value: z.ZodType): unknown {
  return z.toJSONSchema(value, {
    target: "draft-2020-12",
    override: ({ zodSchema, jsonSchema }) => {
      // The portable date-time format validates timestamps without Zod's unbounded pattern.
      if (zodSchema === timestamp) delete jsonSchema.pattern;
    },
  });
}
function method(
  name: string,
  kind: "query" | "command",
  description: string,
  input: z.ZodType,
  output: z.ZodType,
  stream?: z.ZodType,
) {
  return {
    name,
    kind,
    target: "installation",
    description,
    input: schema(input),
    output: schema(output),
    ...(stream ? { stream: schema(stream) } : {}),
  };
}

const descriptor: CapabilityDescriptor = {
  protocol: PROTOCOL,
  reference: "flame.computer.v1",
  composition: "collection",
  scope: "installation",
  methods: [
    method(
      METHODS.read,
      "query",
      "Read the actual Runtime host and current computer-control session. This does not start an engine or authorize computer access.",
      empty,
      snapshot,
    ),
    method(
      METHODS.watch,
      "query",
      "Observe bounded complete snapshots of the computer-control owner. Disconnecting this observation does not close the session.",
      empty,
      z.boolean(),
      snapshot,
    ),
    method(
      METHODS.open,
      "command",
      "Human-only: open one exclusive computer-control session on the Runtime host for the stated lease. Existing sessions are not replaced; expiry cancels and closes the engine.",
      schemas.open,
      session,
    ),
    method(
      METHODS.close,
      "command",
      "Human-only: revoke the exact control session, cancel pending permissions and execution, and join its original engine. Already dispatched external effects cannot be undone.",
      sessionInput,
      session,
    ),
    method(
      METHODS.stop,
      "command",
      "Human-only: interrupt the exact current operation and retire its engine before any later operation can run. A missing original result remains unknown.",
      schemas.stop,
      session,
    ),
    method(
      METHODS.execute,
      "command",
      "Execute js or js_reset on the exact accepted worker and surface. Arguments follow the captured upstream schema. An observed result is durable; transport loss never authorizes replay. Reset discards the original JavaScript heap.",
      execute,
      executionResult,
    ),
    method(
      METHODS.reply,
      "command",
      "Human-only: answer the exact live upstream permission request. The original runtime owns allowed scopes; cancelled, settled or replaced requests cannot be answered.",
      reply,
      z.boolean(),
    ),
    method(
      METHODS.context,
      "query",
      "Read the current worker identity and original runtime instructions and js/js_reset schemas before execution. This never opens a session or grants application access.",
      empty,
      surface,
    ),
    method(
      METHODS.history,
      "query",
      "Read original operation headers in admission order, newest first. This is a projection of durable receipts, not an execution log with a second owner.",
      schemas.history,
      schemas.historyPage,
    ),
    method(
      METHODS.result,
      "query",
      "Read original input, identity and the one observed outcome. An absent outcome means unconfirmed; this method never executes or infers desktop effects.",
      schemas.resultInput,
      record,
    ),
  ],
};

export default descriptor;
