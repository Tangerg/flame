import { QueryObserver } from "@tanstack/react-query";
import type { Disposable, Contributor } from "@/plugins/sdk";
import { queryClient } from "@/lib/queryClient";
import { lookupDataProvider } from "@/plugins/sdk";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import type { AgentSessions } from "@/plugins/builtin/agent/public/services";
import {
  AGENT_SESSIONS_KEY,
  activeSessionWorkspaceSelection,
  subscribeAgentSessionProjection,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import {
  WORKSPACE_RECIPES_KEY,
  type WorkspaceRecipesQuery,
} from "@/plugins/builtin/workspace/public/queries";

interface Recipe {
  name: string;
  description?: string;
  argumentHint?: string;
  body: string;
}

function expandRecipe(body: string, argStr: string): string {
  const trimmed = argStr.trim();
  const parts = trimmed.length ? trimmed.split(/\s+/) : [];
  return body
    .replaceAll("$ARGUMENTS", trimmed)
    .replace(/\$([1-9])(?!\d)/g, (_match, digit: string) => parts[Number(digit) - 1] ?? "");
}

export function recipeWorkspaceQuery(
  activeSessionId: string,
  sessions: readonly AgentSessionSummary[] | undefined,
): WorkspaceRecipesQuery | undefined {
  const selection = activeSessionWorkspaceSelection(activeSessionId, sessions);
  return selection.status === "ready" ? { cwd: selection.cwd } : undefined;
}

function sessionWorkspaceRevision(sessions: readonly AgentSessionSummary[] | undefined): string {
  return JSON.stringify(sessions?.map(({ id, workspace }) => [id, workspace.path]) ?? null);
}

export function installRecipeSlashCommands(
  ctx: Contributor,
  sessionPorts: AgentSessions,
): () => void {
  let dynamic: Disposable[] = [];
  let lastSignature = "";

  const rebuild = (recipes: Recipe[]) => {
    const signature = JSON.stringify(recipes);
    if (signature === lastSignature) return;
    lastSignature = signature;
    for (const disposable of dynamic) disposable.dispose();
    dynamic = recipes.map((recipe) => {
      const label = recipe.description || recipe.name;
      return ctx.contribute(
        SLASH_COMMAND,
        {
          description: recipe.argumentHint ? `${label}  ${recipe.argumentHint}` : label,
          run: ({ args, send }) => send(expandRecipe(recipe.body, args)),
        },
        { key: recipe.name },
      );
    });
  };

  const queryOptions = () => {
    const sessions = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
    const query = recipeWorkspaceQuery(sessionPorts.getActiveSessionId(), sessions);
    return {
      queryKey: [WORKSPACE_RECIPES_KEY, query],
      enabled: query !== undefined,
      staleTime: 60_000,
      queryFn: () => {
        const provider = lookupDataProvider<Recipe[], WorkspaceRecipesQuery>(WORKSPACE_RECIPES_KEY);
        return provider ? provider(query) : Promise.resolve<Recipe[]>([]);
      },
    };
  };
  const observer = new QueryObserver<Recipe[]>(queryClient, queryOptions());
  const unsubscribeRecipes = observer.subscribe((result) => rebuild(result.data ?? []));
  const refresh = () => {
    observer.setOptions(queryOptions());
    rebuild(observer.getCurrentResult().data ?? []);
  };
  refresh();
  const unsubscribeSession = sessionPorts.subscribeActiveSessionId(refresh);
  const unsubscribeQuery = subscribeAgentSessionProjection(sessionWorkspaceRevision, refresh);

  return () => {
    unsubscribeRecipes();
    observer.destroy();
    unsubscribeSession();
    unsubscribeQuery();
    for (const disposable of dynamic) disposable.dispose();
    dynamic = [];
    queryClient.removeQueries({ queryKey: [WORKSPACE_RECIPES_KEY], type: "inactive" });
  };
}
