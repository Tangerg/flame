import { type Contributor } from "@/extensions/definePlugin";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { type FlameClient } from "@flame/runtime-client";
import { HOOKS_KEY, type HooksQuery } from "@/features/hooks/application/hookQueries";
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
