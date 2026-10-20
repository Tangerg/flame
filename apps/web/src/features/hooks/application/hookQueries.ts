type HookScope = "global" | "project";

import { createParameterizedDataQuery } from "@/extensions/dataQuery";
export interface HookReadModel {
  event: string;
  matcher?: string;
  command?: string;
  inject?: string;
  timeoutMs?: number;
  scope: HookScope;
  source: string;
  active: boolean;
}

export interface HookCatalog {
  hooks: HookReadModel[];
  projectRoot?: string;
  projectTrusted: boolean;
}

export interface HooksQuery {
  cwd?: string;
}

export const HOOKS_KEY = "hooks";

export const useHooks = createParameterizedDataQuery<HooksQuery, HookCatalog>(HOOKS_KEY);
