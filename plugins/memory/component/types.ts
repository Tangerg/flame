import { type MutationPromise } from "@flame/runtime-client";
import { type MethodOutputs } from "../package/io.github.tangerg.flame/contracts/methods.generated";
export type MemoryItem = MethodOutputs["flame.memory/list.v2"]["Rows"][number];
export type MemoryPage = MethodOutputs["flame.memory/list.v2"];
export type MutationName =
  | "flame.memory/add.v2"
  | "flame.memory/update.v2"
  | "flame.memory/delete.v2"
  | "flame.memory/review.v2";
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
export type Command = {
  promise: MutationPromise<unknown>;
  // Updates also pin independent records, so the method cannot identify the submitting form.
  source: "editor" | "record";
  intent: CommandIntent;
};
export type Feedback = {
  intent: CommandIntent;
  state: "submitting" | "unknown" | "refused" | "confirmed";
  failure?: { cause: unknown; fallback: "refused" | "submit" | "check" };
};
