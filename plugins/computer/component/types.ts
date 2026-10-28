import type {
  MethodChunks,
  MethodInputs,
  MethodOutputs,
} from "./methods.generated";

export type Snapshot = MethodChunks["flame.computer/watch.v1"];
export type Surface = MethodOutputs["flame.computer/context.v1"];
export type HistoryPage = MethodOutputs["flame.computer/history.v1"];
export type OperationRecord = MethodOutputs["flame.computer/result.v1"];
export type ExecuteInput = MethodInputs["flame.computer/execute.v1"];
export type ReplyInput = MethodInputs["flame.computer/reply.v1"];
export type Approval = NonNullable<
  NonNullable<Snapshot["session"]>["approval"]
>;
