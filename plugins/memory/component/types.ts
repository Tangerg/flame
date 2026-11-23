import { type MutationPromise } from "@flame/runtime-client";
import { type MethodOutputs } from "../package/io.github.tangerg.flame/contracts/methods.generated";
export type MemoryItem = MethodOutputs["flame.memory/list.v3"]["rows"][number];
export type MemoryPage = MethodOutputs["flame.memory/list.v3"];
export type MutationName =
  | "flame.memory/add.v3"
  | "flame.memory/update.v3"
  | "flame.memory/delete.v3"
  | "flame.memory/review.v3";
export type Scope = "project" | "user";
export type Accepted = {
  page: MemoryPage;
  cursors: string[];
  scope: Scope;
  cwd: string;
};
export type Read = { loading: boolean; accepted?: Accepted; error?: string };
export type Edit = (
  | { type: "add"; content: string }
  | { type: "update"; item: MemoryItem; content: string }
  | { type: "delete"; item: MemoryItem }
) & {
  opener: HTMLElement;
};
export type CommandIntent =
  | { action: "add"; scope: Scope }
  | {
      action: "edit" | "delete" | "approve" | "reject" | "pin" | "unpin";
      id: string;
    };
export type CommandSource = "editor" | "record";
export type Command = {
  promise: MutationPromise<unknown>;
  // Updates also pin independent records, so the method cannot identify the submitting form.
  source: CommandSource;
  intent: CommandIntent;
};
export type Failure = {
  cause: unknown;
  fallback: "refused" | "submit" | "check";
};
// One command at a time. Checking retries an unknown settlement through the
// original command, so its idempotency key stays visible until it settles.
export type CommandState =
  | { phase: "idle" }
  | { phase: "submitting"; command: Command }
  | { phase: "checking"; command: Command }
  | { phase: "unknown"; command: Command; failure?: Failure };
export type Outcome = {
  intent: CommandIntent;
  state: "refused" | "confirmed";
  failure?: Failure;
};
export type Feedback = {
  intent: CommandIntent;
  state: "submitting" | "unknown" | Outcome["state"];
  failure?: Failure;
};
