import { type ComponentContext } from "@flame/component-sdk";
import { type MutationPromise } from "@flame/runtime-client";
import { type Model } from "@flame/runtime-client/wire";
import {
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";
import { METHODS } from "./methods";
export type Schedule = MethodOutputs[typeof METHODS.list]["rows"][number];
export type Page = MethodOutputs[typeof METHODS.list];
export type Draft = MethodInputs[typeof METHODS.create];
export type Mutation = (typeof METHODS)[
  "create" | "update" | "delete" | "runNow"];
export type Command = {
  method: Mutation;
  promise: MutationPromise<unknown>;
  label: string;
};
// One command at a time. An uncertain settlement keeps its original command
// until a check through the same idempotency key settles it.
export type CommandState =
  | { phase: "submitting"; command: Command }
  | { phase: "uncertain"; command: Command; error: string };
export type Notice = string | MethodOutputs[typeof METHODS.runNow];
export type Edit = { basis?: Schedule; draft: Draft };
export type ModelRead = {
  source: Pick<ComponentContext, "runtime" | "track" | "signal">;
  catalogs: Map<string, Model[]>;
  loading: boolean;
  error?: string;
};
