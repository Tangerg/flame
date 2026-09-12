import { useState } from "react";
import { Segmented } from "@/ui";
import { useT } from "@/lib/i18n";
import { AvailableSkills } from "./views/AvailableSkills";
import { SkillLibrary } from "./views/SkillLibrary";
import { SkillProposals } from "./views/SkillProposals";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";

type SkillSection = "available" | "review" | "library";

export function SkillsTab() {
  const t = useT();
  const [section, setSection] = useState<SkillSection>("available");
  const actions = (
    <Segmented
      value={section}
      onChange={setSection}
      ariaLabel={t("skills.title")}
      options={[
        { value: "available", label: t("skills.tab.available") },
        { value: "review", label: t("skills.tab.review") },
        { value: "library", label: t("skills.tab.library") },
      ]}
    />
  );

  return (
    <WorkspaceViewLayout icon="sparkle" title="skills.title" actions={actions}>
      {section === "available" && <AvailableSkills />}
      {section === "review" && <SkillProposals />}
      {section === "library" && <SkillLibrary />}
    </WorkspaceViewLayout>
  );
}
