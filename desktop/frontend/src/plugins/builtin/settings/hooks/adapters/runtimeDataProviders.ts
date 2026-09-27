import type { Contributor } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import type { FlameClient } from "@flame/runtime-contract/client";
import { HOOKS_KEY, type HooksQuery } from "../application/hookQueries";

export function registerHookDataProviders(
  ctx: Contributor,
  runtimeClient: () => FlameClient,
): void {
  ctx.contribute(DATA_PROVIDER, {
    key: HOOKS_KEY,
    fetcher: async (params, signal) => {
      const cwd = (params as HooksQuery | undefined)?.cwd;
      const resources = await runtimeClient().workspaces.open(
        cwd ? { path: cwd } : undefined,
        signal,
      );
      return resources.hooks.list(signal);
    },
  });
}
