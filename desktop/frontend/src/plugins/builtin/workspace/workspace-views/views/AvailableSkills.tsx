import * as stylex from "@stylexjs/stylex";
import { DataView, Tag, vocab } from "@/ui";
import { type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./viewStyles";
import { useT } from "@/lib/i18n";
import { useWorkspaceSkills } from "@/plugins/builtin/workspace/application/workspaceQueries";
import { useWorkspaceCapability } from "@/plugins/builtin/workspace/application/workspaceCapabilities";
import { workspaceSkillsViewModel } from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";

export function AvailableSkills() {
  const t = useT();
  const skillsEnabled = useWorkspaceCapability("skills");
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, isError, refetch } = useWorkspaceSkills(
    workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
  const view = workspaceSkillsViewModel(data ?? [], skillsEnabled);

  return (
    <>
      <div {...stylex.props(vs.gutter, vs.rowPad, typeStep.uiSm, vocab.muted)}>
        {view.enabled ? t("skills.available", { count: view.count }) : t("skills.off")}
      </div>
      <DataView
        items={view.rows}
        isLoading={view.enabled && (isLoading || workspace.status === "resolving")}
        isError={isError}
        onRetry={refetch}
        skeletonCount={4}
        empty={
          skillsEnabled
            ? {
                icon: "sparkle",
                title: t("skills.empty.title"),
                sub: t("skills.empty.sub"),
              }
            : {
                icon: "sparkle",
                title: t("skills.disabled.title"),
                sub: t("skills.disabled.sub"),
              }
        }
      >
        {(rows) => (
          <div {...stylex.props(vocab.column)}>
            {rows.map((s) => (
              <div key={s.id} {...stylex.props(vs.gutter, vs.rowPad)}>
                <div {...stylex.props(vocab.line, vocab.min)}>
                  <div {...stylex.props(vs.title, vocab.truncate, typeStep.uiMd)}>{s.name}</div>
                  {s.scope && <Tag>{s.scope}</Tag>}
                </div>
                {s.description && (
                  <div {...stylex.props(vs.description, typeStep.uiSm)}>{s.description}</div>
                )}
              </div>
            ))}
          </div>
        )}
      </DataView>
    </>
  );
}
