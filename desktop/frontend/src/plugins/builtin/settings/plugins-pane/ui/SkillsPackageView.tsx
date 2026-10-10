import { useMemo, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import {
  SkillInspectionControl,
  SkillLibraryControls,
  SkillProposalControls,
} from "@/plugins/builtin/workspace/public/skills";
import { Segmented, SystemMessage, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { space } from "@/styles/tokens.stylex";
import type {
  PluginViewReads,
  SkillViewTarget,
  SkillViewSnapshot,
} from "../application/pluginView";
import { PackageView } from "./PackageView";

const styles = stylex.create({
  root: { height: "100%", minHeight: 0 },
  controls: { padding: space.s3, flexShrink: 0 },
  page: { flex: 1, minHeight: 0 },
});
type Section = "available" | "review" | "library";
type CatalogTarget = Exclude<SkillViewTarget, { type: "detail" }>;
type Props = {
  reads(target: SkillViewTarget): PluginViewReads<SkillViewSnapshot>;
  title: string;
  lifetime: ContributionLifetime;
  carrier: PluginCarrier;
};
export function SkillsPackageView(props: Props) {
  const t = useT();
  const [section, setSection] = useState<Section>("available");
  const workspace = useActiveSessionWorkspace();
  const available = useRuntimeCapability("skills");
  const cwd = section !== "library" && workspace.status === "ready" ? workspace.cwd : undefined;
  const context = useMemo<CatalogTarget | undefined>(
    () =>
      section === "library"
        ? { type: section }
        : cwd
          ? { type: section, workspace: { path: cwd } }
          : undefined,
    [section, cwd],
  );
  return (
    <div {...stylex.props(vocab.column, styles.root)}>
      <div {...stylex.props(styles.controls)}>
        <Segmented
          value={section}
          onChange={setSection}
          ariaLabel={t("skills.title")}
          options={(["available", "review", "library"] as const).map((value) => ({
            value,
            label: t(`skills.tab.${value}`),
          }))}
        />
      </div>
      {!available ? (
        <SystemMessage>{t("skills.disabled.title")}</SystemMessage>
      ) : !context ? (
        <SystemMessage>
          {t(workspace.status === "resolving" ? "packages.view.loading" : "skills.noProject")}
        </SystemMessage>
      ) : (
        <BoundSkillsView
          key={`${context.type}:${context.type === "library" ? "" : context.workspace.path}`}
          {...props}
          context={context}
        />
      )}
    </div>
  );
}
function BoundSkillsView({
  context,
  reads: createReads,
  ...props
}: Props & { context: CatalogTarget }) {
  const [name, setName] = useState<string>();
  const [refreshVersion, setRefreshVersion] = useState(0);
  const target = useMemo<SkillViewTarget>(
    () =>
      context.type === "available" && name
        ? { type: "detail", workspace: context.workspace, name }
        : context,
    [context, name],
  );
  const reads = useMemo(() => createReads(target), [createReads, target]);
  const saved = () => setRefreshVersion((value) => value + 1);
  return (
    <>
      {context.type === "available" && (
        <SkillInspectionControl cwd={context.workspace.path} name={name} onInspect={setName} />
      )}
      {context.type === "library" && (
        <SkillLibraryControls signal={props.lifetime.signal} onSaved={saved} />
      )}
      {context.type === "review" && (
        <SkillProposalControls
          cwd={context.workspace.path}
          signal={props.lifetime.signal}
          onSaved={saved}
        />
      )}
      <div {...stylex.props(styles.page)}>
        <PackageView key={refreshVersion} {...props} reads={reads} />
      </div>
    </>
  );
}
