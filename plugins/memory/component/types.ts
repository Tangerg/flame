import { type MutationPromise } from "@flame/runtime-client";
import { type MethodOutputs } from "../package/io.github.tangerg.flame/contracts/methods.generated";
export type MemoryItem = MethodOutputs["flame.memory/list.v1"]["Rows"][number];
export type MemoryPage = MethodOutputs["flame.memory/list.v1"];
export type MutationName =
  | "flame.memory/add.v1"
  | "flame.memory/update.v1"
  | "flame.memory/delete.v1"
  | "flame.memory/review.v1";
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
export type Command = {
  promise: MutationPromise<unknown>;
  // Updates also pin independent records, so the method cannot identify the submitting form.
  source: "editor" | "record";
  label: string;
};
export type Feedback = {
  label: string;
  state: "submitting" | "unknown" | "refused" | "confirmed";
  detail?: string;
};
