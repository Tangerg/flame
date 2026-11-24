import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { DataView } from "@/ui/index";
import { SectionLabel, TextButton } from "@/ui/index";
import { IconButton, vocab } from "@flame/ui";
import { AgentWorkIndexGroupList } from "@/ui/agent/index";
import { ProjectRow } from "@/features/sidebar/ui/ProjectRow";
import { SessionList } from "@/features/sidebar/ui/SessionList";
import { useT } from "@/lib/i18n/index";
import { type WorkProjectGroup } from "@/features/navigation/public/workIndex";
import { type WorkIndexActions } from "@/features/navigation/public/workIndex";
import { contributeWorkIndexItem } from "@/features/navigation/public/workIndex";
import { useWorkIndex } from "@/features/navigation/public/workIndex";
import { useWorkIndexActions } from "@/features/navigation/public/workIndex";
import { workGroupHasRunning, workGroupWaitingCount } from "@/features/navigation/public/workIndex";
import {
  ProjectNameDialog,
  useProjects,
  useProjectsAccess,
  useShownProjectHome,
  useStartProjectSession,
} from "@/features/projects/public/projects";
import { definePlugin } from "@/extensions/definePlugin";
import { space } from "@flame/ui/tokens.stylex";
const pj = stylex.create({
  label: { paddingLeft: space.s2, paddingRight: space.s1, paddingTop: 0, paddingBottom: space.s2 },
  archived: { paddingInline: space.s2, paddingBlock: space.s1 },
});

function ProjectGroupNode({
  group,
  actions,
  shown,
  activeSessionId,
}: {
  group: WorkProjectGroup;
  actions: WorkIndexActions;
  shown: boolean;
  activeSessionId: string;
}) {
  const access = useProjectsAccess();
  const startSession = useStartProjectSession();
  const holdsActive = group.sessions.some((session) => session.id === activeSessionId);
  const [open, setOpen] = useState(holdsActive);
  const [revealedFor, setRevealedFor] = useState(activeSessionId);
  if (revealedFor !== activeSessionId) {
    setRevealedFor(activeSessionId);
    if (holdsActive && !open) setOpen(true);
  }
  return (
    <div {...stylex.props(vocab.column)}>
      <ProjectRow
        project={group.project}
        active={shown}
        open={open}
        count={group.sessions.length}
        waiting={workGroupWaitingCount(group)}
        running={workGroupHasRunning(group)}
        onOpen={() => {
          // Reopening the Project already on screen folds its Sessions instead.
          if (shown) setOpen((value) => !value);
          else {
            setOpen(true);
            access.openHome(group.project.id);
          }
        }}
        onNewSession={() => startSession(group.project.id)}
        canCreateSession={actions.canCreateSessionInFolder}
      />
      {open && group.sessions.length > 0 && (
        <SessionList
          sessions={group.sessions}
          actions={actions}
          activeSessionId={activeSessionId}
          indented
          showTime={false}
        />
      )}
    </div>
  );
}

function ProjectsSection() {
  const t = useT();
  const workIndex = useWorkIndex();
  const actions = useWorkIndexActions();
  const projects = useProjects();
  const shown = useShownProjectHome();
  const [creating, setCreating] = useState(false);
  const [showArchived, setShowArchived] = useState(false);
  const groups = workIndex.projects;
  const active = groups?.filter((group) => !group.project.archived);
  const archived = groups?.filter((group) => group.project.archived) ?? [];
  const access = useProjectsAccess();

  const render = (items: readonly WorkProjectGroup[]) => (
    <AgentWorkIndexGroupList>
      {items.map((group) => (
        <ProjectGroupNode
          key={group.project.id}
          group={group}
          actions={actions}
          shown={shown === group.project.id}
          activeSessionId={workIndex.activeSessionId}
        />
      ))}
    </AgentWorkIndexGroupList>
  );

  return (
    <>
      <SectionLabel
        className={stylex.props(pj.label).className}
        trailing={
          <IconButton
            icon={iconGlyph("plus")}
            size="xs"
            quiet
            aria-label={t("projects.create.title")}
            disabled={!actions.canCreateSessionInFolder}
            onClick={() => setCreating(true)}
          />
        }
      >
        {t("workIndex.section.projects")}
      </SectionLabel>
      <DataView
        items={active}
        isLoading={projects.isPending && groups === undefined}
        failure={projects.isError && groups === undefined ? projects.error : undefined}
        onRetry={() => void projects.refetch()}
        skeletonCount={2}
        skeletonVariant="compact"
        loadingLabel={t("common.loading")}
        empty={{ title: t("projects.empty.title"), size: "compact" }}
        error={{ title: t("projects.error.title"), size: "compact" }}
      >
        {render}
      </DataView>
      {archived.length > 0 && (
        <TextButton
          type="button"
          shape="row"
          size="xs"
          tone="faint"
          aria-expanded={showArchived}
          onClick={() => setShowArchived((open) => !open)}
          className={stylex.props(pj.archived).className}
        >
          {t("projects.archived", { count: archived.length })}
        </TextButton>
      )}
      {showArchived && archived.length > 0 && render(archived)}
      {creating && (
        <ProjectNameDialog
          onClose={() => setCreating(false)}
          onSaved={(project) => access.openHome(project.id)}
        />
      )}
    </>
  );
}

export const sidebarProjects = definePlugin({
  name: "flame.builtin.sidebar-projects",
  setup(ctx) {
    contributeWorkIndexItem(ctx.contributions, {
      id: "projects",
      order: -5,
      component: ProjectsSection,
    });
  },
});
