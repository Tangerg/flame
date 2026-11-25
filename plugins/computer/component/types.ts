import type {
  MethodChunks,
  MethodInputs,
  MethodOutputs,
} from "./methods.generated";
import type { METHODS } from "../contracts/methods";

export type Snapshot = MethodChunks[typeof METHODS.watch];
export type Surface = MethodOutputs[typeof METHODS.context];
export type HistoryPage = MethodOutputs[typeof METHODS.history];
export type OperationRecord = MethodOutputs[typeof METHODS.result];
export type ExecuteInput = MethodInputs[typeof METHODS.execute];
export type ReplyInput = MethodInputs[typeof METHODS.reply];
export type Approval = NonNullable<
  NonNullable<Snapshot["session"]>["approval"]
>;
