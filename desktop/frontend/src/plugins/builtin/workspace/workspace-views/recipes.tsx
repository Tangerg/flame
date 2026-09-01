import { DataView } from "@/ui";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useWorkspaceRecipes } from "@/plugins/builtin/workspace/application/workspaceQueries";
import { workspaceRecipesViewModel } from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";

export function RecipesTab() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, isError } = useWorkspaceRecipes(
    workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
  const view = workspaceRecipesViewModel(data ?? []);

  return (
    <WorkspaceViewLayout
      icon="command"
      titleStrong
      title="recipes.title"
      sub={t("recipes.available", { count: view.count })}
      scrollClassName="py-1"
    >
      <DataView
        items={view.rows}
        isLoading={isLoading || workspace.status === "resolving"}
        isError={isError}
        skeletonCount={4}
        empty={{ icon: "command", title: t("recipes.empty.title"), sub: t("recipes.empty.sub") }}
      >
        {(rows) => (
          <div className="flex flex-col">
            {rows.map((r) => (
              <div key={r.id} className="px-[var(--density-view-gutter)] py-2">
                <div className="flex items-center gap-2">
                  <span className="truncate font-mono text-ui-md font-semibold text-accent">
                    {r.command}
                  </span>
                  {r.argumentHint && (
                    <span className="truncate font-mono text-ui-sm text-fg-faint">
                      {r.argumentHint}
                    </span>
                  )}
                  <span className="ml-auto shrink-0 rounded-sm bg-surface-2 px-1.5 py-px font-mono text-ui-xs text-fg-faint">
                    {r.scope}
                  </span>
                </div>
                {r.description && (
                  <div className="mt-0.5 text-ui-sm leading-body text-fg-muted">
                    {r.description}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}
