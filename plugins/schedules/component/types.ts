import { type ComponentContext } from "@flame/component-sdk";
import { type MutationPromise } from "@flame/runtime-client";
import { type Model } from "@flame/runtime-client/wire";
import {
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";
export type Schedule = MethodOutputs["flame.schedules/list.v1"]["Rows"][number];
export type Page = MethodOutputs["flame.schedules/list.v1"];
export type Draft = MethodInputs["flame.schedules/create.v1"];
export type Mutation =
  | "flame.schedules/create.v1"
  | "flame.schedules/update.v1"
  | "flame.schedules/delete.v1"
  | "flame.schedules/runNow.v1";
export type Command = {
  promise: MutationPromise<unknown>;
  label: string;
  uncertain: boolean;
  error?: string;
};
export type Read = {
  loading: boolean;
  accepted?: { page: Page; history: string[] };
  error?: string;
};
export type Edit = { basis?: Schedule; draft: Draft };
export type ModelRead = {
  context: ComponentContext;
  catalogs: Map<string, Model[]>;
  loading: boolean;
  error?: string;
};
