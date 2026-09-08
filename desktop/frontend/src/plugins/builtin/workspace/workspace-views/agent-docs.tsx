import * as stylex from "@stylexjs/stylex";
import { Badge, DataView } from "@/ui";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useT } from "@/lib/i18n";
import { face, type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
import { useWorkspaceAgentDocs } from "@/plugins/builtin/workspace/application/workspaceQueries";
import { workspaceAgentDocsViewModel } from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";

export function AgentDocsTab() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, isError, refetch } = useWorkspaceAgentDocs(
    workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
  const view = workspaceAgentDocsViewModel(data ?? []);

  return (
    <WorkspaceViewLayout
      icon="book"
      titleStrong
      title="agentDocs.title"
      sub={t("agentDocs.found", { count: view.count })}
      scrollClassName="py-1"
    >
      <DataView
        items={view.rows}
        isLoading={isLoading || workspace.status === "resolving"}
        isError={isError}
        onRetry={refetch}
        skeletonCount={3}
        empty={{
          icon: "book",
          title: t("agentDocs.empty.title"),
          sub: t("agentDocs.empty.sub"),
        }}
      >
        {(rows) => (
          <div {...stylex.props(vs.stack)}>
            {rows.map((d) => (
              <div key={d.id} {...stylex.props(vs.splitLine, vs.gutter, vs.rowPad)}>
                <div {...stylex.props(vs.min)}>
                  <div {...stylex.props(vs.title, vs.truncate, typeStep.uiMd)}>{d.title}</div>
                  <div {...stylex.props(vs.subCaption, vs.truncate, typeStep.uiSm, face.mono)}>
                    {d.path}
                  </div>
                </div>
                <Badge>{t(d.scopeLabelKey)}</Badge>
              </div>
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}
