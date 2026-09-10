import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useCallback, useRef, useState } from "react";
import { DataView, gap, PillButton, SectionLabel, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
import { notifyError } from "@/plugins/sdk";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import {
  useManagedSkills,
  type ManagedSkill,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import { archiveSkill, restoreSkill } from "@/plugins/builtin/workspace/application/skillCuration";

export function SkillLibraryTab() {
  const t = useT();
  const { data, isLoading, isError, refetch } = useManagedSkills();
  const skills = data ?? [];
  const activeCount = skills.filter((s) => s.lifecycle === "active").length;

  return (
    <WorkspaceViewLayout
      icon="sparkle"
      title="skillLibrary.title"
      sub={t("skillLibrary.sub", { active: activeCount, archived: skills.length - activeCount })}
    >
      <DataView
        items={skills}
        isLoading={isLoading}
        isError={isError}
        onRetry={refetch}
        skeletonCount={4}
        empty={{
          icon: "sparkle",
          title: t("skillLibrary.empty.title"),
          sub: t("skillLibrary.empty.sub"),
        }}
      >
        {(rows) => {
          const active = rows.filter((s) => s.lifecycle === "active");
          const archived = rows.filter((s) => s.lifecycle === "archived");
          return (
            <div {...stylex.props(vocab.column, gap.s4, vs.padBlockSm)}>
              {active.length > 0 && (
                <SkillSection label={t("skillLibrary.section.active")} skills={active} />
              )}
              {archived.length > 0 && (
                <SkillSection label={t("skillLibrary.section.archived")} skills={archived} />
              )}
            </div>
          );
        }}
      </DataView>
    </WorkspaceViewLayout>
  );
}

function SkillSection({ label, skills }: { label: string; skills: ManagedSkill[] }) {
  return (
    <div {...stylex.props(vocab.column)}>
      <div {...stylex.props(vs.gutter, vs.sectionPad)}>
        <SectionLabel className={stylex.props(vs.sectionLabel).className}>{label}</SectionLabel>
      </div>
      {skills.map((skill) => (
        <SkillRow key={skill.name} skill={skill} />
      ))}
    </div>
  );
}

function SkillRow({ skill }: { skill: ManagedSkill }) {
  const t = useT();
  const archived = skill.lifecycle === "archived";
  const actionPending = useRef(false);
  const [busy, setBusy] = useState(false);
  const onAction = useCallback(async () => {
    if (actionPending.current) return;
    actionPending.current = true;
    setBusy(true);
    try {
      await (archived ? restoreSkill(skill.name) : archiveSkill(skill.name));
    } catch (error) {
      if (!wasGenerationRetired(error)) {
        notifyError(error instanceof Error ? error.message : t("skillLibrary.error"), {
          source: "skills",
        });
      }
    } finally {
      actionPending.current = false;
      setBusy(false);
    }
  }, [archived, skill.name, t]);

  return (
    <div {...stylex.props(vs.lineTop, vs.gutter, vs.rowPad)}>
      <div {...stylex.props(vocab.fill)}>
        <div {...stylex.props(vs.title, vocab.truncate, typeStep.uiMd)}>{skill.name}</div>
        {skill.description && (
          <div {...stylex.props(vs.description, typeStep.uiSm)}>{skill.description}</div>
        )}
      </div>
      <PillButton
        size="sm"
        variant={archived ? "outlined" : "danger"}
        pending={busy}
        onClick={() => void onAction()}
      >
        {archived ? t("skillLibrary.restore") : t("skillLibrary.archive")}
      </PillButton>
    </div>
  );
}
