import * as stylex from "@stylexjs/stylex";
import { DataView, Tag, vocab } from "@/ui";
import { face, type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
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
          <div {...stylex.props(vocab.column)}>
            {rows.map((r) => (
              <div key={r.id} {...stylex.props(vs.gutter, vs.rowPad)}>
                <div {...stylex.props(vocab.line, vocab.min)}>
                  {/* The command's own name, in the ink the command menu gives it. Accent here
                      measured 3.4:1 on this surface in dark — an emphasis that costs the
                      reader the thing being emphasised. Mono and semibold already say
                      "something you can run". */}
                  <span {...stylex.props(vs.title, vocab.truncate, typeStep.uiMd, face.mono)}>
                    {r.command}
                  </span>
                  {r.argumentHint && (
                    <span {...stylex.props(vocab.faint, vocab.truncate, typeStep.uiSm, face.mono)}>
                      {r.argumentHint}
                    </span>
                  )}
                  <Tag {...stylex.props(vs.pushEnd)}>{r.scope}</Tag>
                </div>
                {r.description && (
                  <div {...stylex.props(vs.description, typeStep.uiSm)}>{r.description}</div>
                )}
              </div>
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}
