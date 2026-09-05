import { DataView, Tag } from "@/ui";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useWorkspaceRecipes } from "@/plugins/builtin/workspace/application/workspaceQueries";
import { workspaceRecipesViewModel } from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";

export function RecipesTab() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, isError, refetch } = useWorkspaceRecipes(
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
        onRetry={refetch}
        skeletonCount={4}
        empty={{ icon: "command", title: t("recipes.empty.title"), sub: t("recipes.empty.sub") }}
      >
        {(rows) => (
          <div className="flex flex-col">
            {rows.map((r) => (
              <div key={r.id} className="px-[var(--density-column-gutter-wide)] py-2">
                <div className="flex items-center gap-2">
                  {/* The command's own name, in the ink the command menu gives it. Accent here
                      measured 3.4:1 on this surface in dark — an emphasis that costs the
                      reader the thing being emphasised. Mono and semibold already say
                      "something you can run". */}
                  <span className="truncate font-mono text-ui-md font-semibold text-fg">
                    {r.command}
                  </span>
                  {r.argumentHint && (
                    <span className="truncate font-mono text-ui-sm text-fg-faint">
                      {r.argumentHint}
                    </span>
                  )}
                  <Tag className="ml-auto">{r.scope}</Tag>
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
