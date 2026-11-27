import { type MethodOutputs } from "../package/io.github.tangerg.flame/contracts/methods.generated";
import { METHODS } from "./methods";
export type MemoryItem = MethodOutputs[typeof METHODS.list]["rows"][number];
export type MemoryPage = MethodOutputs[typeof METHODS.list];
export type MutationName = (typeof METHODS)[
  "add" | "update" | "delete" | "review"];
export type Scope = "project" | "user";
export type Accepted = {
  page: MemoryPage;
  cursors: string[];
  scope: Scope;
  target: string;
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
export type CommandPresentation = {
  source: CommandSource;
  intent?: CommandIntent;
};
export type Failure = {
  cause: unknown;
  fallback: "refused" | "submit" | "check";
};
export type Outcome = {
  intent?: CommandIntent;
  state: "refused" | "confirmed";
  failure?: Failure;
};
export type Feedback = {
  intent?: CommandIntent;
  state: "submitting" | "unknown" | Outcome["state"];
  failure?: Failure;
};
