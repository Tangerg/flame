import { iconGlyph } from "@/ui/icons/index";
import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { DataView } from "@/ui/index";
import { SectionLabel } from "@/ui/index";
import { IconButton, vocab } from "@flame/ui";
import { AgentWorkIndexGroupList } from "@/ui/agent/index";
import { FolderRow } from "@/features/sidebar/ui/FolderRow";
import { SessionList } from "@/features/sidebar/ui/SessionList";
import { useT } from "@/lib/i18n/index";
import { type WorkGroup, type WorkFolder } from "@/features/navigation/public/workIndex";
import { type WorkIndexActions } from "@/features/navigation/public/workIndex";
import { contributeWorkIndexItem } from "@/features/navigation/public/workIndex";
import { useWorkIndex } from "@/features/navigation/public/workIndex";
import { useWorkIndexActions } from "@/features/navigation/public/workIndex";
import { workGroupHasRunning, workGroupWaitingCount } from "@/features/navigation/public/workIndex";
import { definePlugin } from "@/extensions/definePlugin";
import { space } from "@flame/ui/tokens.stylex";
const pj = stylex.create({
  label: { paddingLeft: space.s2, paddingRight: space.s1, paddingTop: 0, paddingBottom: space.s2 },
});

function FolderGroupNode({
  group,
  actions,
  activeCwd,
  activeSessionId,
  onNewSession,
}: {
  group: WorkGroup;
  actions: WorkIndexActions;
  activeCwd: string | undefined;
  activeSessionId: string;
  onNewSession: (folder: WorkFolder) => void;
}) {
  const [open, setOpen] = useState(true);
  const holdsActive = group.sessions.some((session) => session.id === activeSessionId);
  const [revealedFor, setRevealedFor] = useState(activeSessionId);
  if (revealedFor !== activeSessionId) {
    setRevealedFor(activeSessionId);
    if (holdsActive && !open) setOpen(true);
  }
  const waiting = workGroupWaitingCount(group);
  const running = workGroupHasRunning(group);

  return (
    <div {...stylex.props(vocab.column)}>
      <FolderRow
        folder={group.folder}
        active={group.folder.id === activeCwd && !open}
        open={open}
        count={group.sessions.length}
        waiting={waiting}
        running={running}
        onToggle={() => setOpen((v) => !v)}
        onNewSession={onNewSession}
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

function FoldersSection() {
  const t = useT();
  const workIndex = useWorkIndex();
  const actions = useWorkIndexActions();

  return (
    <>
      <SectionLabel
        className={stylex.props(pj.label).className}
        trailing={
          <IconButton
            icon={iconGlyph("plus")}
            size="xs"
            quiet
            aria-label={t("workIndex.action.addFolder")}
            disabled={!actions.canCreateSessionInFolder}
            onClick={actions.chooseSessionFolder}
          />
        }
      >
        {t("workIndex.section.folders")}
      </SectionLabel>
      <DataView
        items={workIndex.groups}
        isLoading={workIndex.isLoading}
        failure={workIndex.error}
        onRetry={workIndex.retry}
        skeletonCount={3}
        skeletonVariant="compact"
        loadingLabel={t("common.loading")}
        // Once work lives in Projects, an empty folder list is expected, not a prompt.
        empty={
          workIndex.projects?.length
            ? undefined
            : { title: t("folders.empty.title"), sub: t("folders.empty.sub"), size: "compact" }
        }
        error={{
          title: t("folders.error.title"),
          sub: t("folders.error.sub"),
          size: "compact",
        }}
      >
        {(items) => (
          <AgentWorkIndexGroupList>
            {items.map((group) => (
              <FolderGroupNode
                key={group.folder.id}
                group={group}
                actions={actions}
                activeCwd={workIndex.activeCwd}
                activeSessionId={workIndex.activeSessionId}
                onNewSession={(folder) => actions.startSessionInFolder(folder.id)}
              />
            ))}
          </AgentWorkIndexGroupList>
        )}
      </DataView>
    </>
  );
}

export const sidebarFolders = definePlugin({
  name: "flame.builtin.sidebar-folders",
  setup(ctx) {
    contributeWorkIndexItem(ctx.contributions, {
      id: "folders",
      order: 0,
      component: FoldersSection,
    });
  },
});
