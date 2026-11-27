import { type ComponentContext } from "@flame/component-sdk";
import { type Model } from "@flame/runtime-client/wire";
import {
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";
import { METHODS, type AuthoringMethod } from "./methods";
export type Schedule = MethodOutputs[typeof METHODS.list]["rows"][number];
export type Page = MethodOutputs[typeof METHODS.list];
export type Draft = MethodInputs[typeof METHODS.create];
export type Command = {
  method: AuthoringMethod;
  label: string;
};
export type Notice = string | MethodOutputs[typeof METHODS.runNow];
export type Edit = { basis?: Schedule; draft: Draft };
export type ModelRead = {
  source: Pick<ComponentContext, "runtime" | "track" | "signal">;
  catalogs: Map<string, Model[]>;
  loading: boolean;
  error?: string;
};
