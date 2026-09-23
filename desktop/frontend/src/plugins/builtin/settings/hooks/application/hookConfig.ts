import { useHooks, type HookReadModel, type HooksQuery } from "./hookQueries";

export type { HookReadModel };

interface HookListViewModel {
  hooks: HookReadModel[];
  projectRoot?: string;
  projectTrusted: boolean;
  hasProjectHooks: boolean;
}

export function useHookConfigs(input: HooksQuery | undefined) {
  const query = useHooks(input);
  const source = query.data;
  const data: HookListViewModel | undefined = source
    ? {
        hooks: source.hooks.map((hook) => ({
          event: hook.event,
          matcher: hook.matcher,
          command: hook.command,
          inject: hook.inject,
          timeoutMs: hook.timeoutMs,
          scope: hook.scope,
          source: hook.source,
          active: hook.active,
        })),
        projectRoot: source.projectRoot,
        projectTrusted: source.projectTrusted,
        hasProjectHooks: source.hooks.some((hook) => hook.scope === "project"),
      }
    : undefined;
  return { ...query, data };
}
