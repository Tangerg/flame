import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { basename } from "@/lib/path";
import { Trans, useT } from "@/lib/i18n";
import {
  useWorkIndex,
  useWorkIndexActions,
  type WorkGroup,
} from "@/plugins/builtin/navigation/public/workIndex";
import { Button, DropdownMenu, Icon, vocab } from "@/ui";
import { AgentComposerTopTraySurface } from "@/ui/agent";
import { color, space, type as typeStep, weight } from "@/styles/tokens.stylex";

interface ProjectMenuContentProps {
  groups: readonly WorkGroup[] | undefined;
  activeCwd: string | undefined;
  loading: boolean;
  canCreate: boolean;
  onSelect: (cwd: string) => void;
  onAdd: () => void;
  align: "start" | "center";
}

const pl = stylex.create({
  // Wide enough for a path, and never wider than the window with its own margin left over.
  menu: { width: "min(320px, calc(100vw - 32px))" },
  heading: {
    paddingInline: space.s2,
    paddingTop: space.s1,
    paddingBottom: space.s1_5,
    color: color.fgFaint,
    fontWeight: weight.medium,
  },
  item: { paddingInline: space.s2 },
  tray: { display: "flex", minWidth: 0, alignItems: "center" },
  // The tray tucks UNDER the composer: it is inset from the composer's edges, overlaps it by
  // 18px, and pads its own bottom past that overlap so its content clears the composer's top
  // edge. No blur of its own — the composer above it already carries one, and two stacked
  // read as a smear where they overlap.
  traySurface: {
    position: "relative",
    top: space.s1,
    zIndex: 0,
    marginInline: space.s3,
    marginBottom: "-18px",
    width: "calc(100% - 24px)",
    backgroundColor: "var(--app-composer-project-tray-surface)",
    paddingInline: space.s1_5,
    paddingTop: space.s1_5,
    paddingBottom: "27px",
    backdropFilter: "none",
    WebkitBackdropFilter: "none",
  },
  chooseLabel: { maxWidth: "240px" },
  full: { maxWidth: "100%" },
});

function ProjectMenuContent({
  groups,
  activeCwd,
  loading,
  canCreate,
  onSelect,
  onAdd,
  align,
}: ProjectMenuContentProps) {
  const t = useT();
  return (
    <DropdownMenu.Content
      side="top"
      align={align}
      sideOffset={8}
      className={stylex.props(pl.menu).className}
    >
      <div {...stylex.props(pl.heading, typeStep.uiXs)}>{t("composer.project.select")}</div>
      {loading && !groups ? (
        <DropdownMenu.Item disabled layout="glyph" className={stylex.props(pl.item).className}>
          <Icon name="folder" size="sm" className={stylex.props(vocab.faint).className} />
          <span>{t("common.loading")}</span>
        </DropdownMenu.Item>
      ) : (
        groups?.map(({ project }) => (
          <DropdownMenu.Item
            key={project.id}
            disabled={project.cwdMissing || !canCreate}
            onClick={() => {
              if (project.id !== activeCwd) onSelect(project.id);
            }}
            title={project.cwdMissing ? t("project.row.missing") : project.id}
            layout="pick"
            className={stylex.props(pl.item).className}
          >
            <Icon name="folder" size="sm" className={stylex.props(vocab.muted).className} />
            <span {...stylex.props(vocab.min, vocab.truncate)}>{project.name}</span>
            {project.id === activeCwd ? (
              <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
            ) : (
              <span aria-hidden />
            )}
          </DropdownMenu.Item>
        ))
      )}
      <DropdownMenu.Separator />
      <DropdownMenu.Item
        disabled={!canCreate}
        onClick={onAdd}
        layout="glyph"
        className={stylex.props(pl.item).className}
      >
        <Icon name="plus" size="sm" className={stylex.props(vocab.muted).className} />
        <span>{t("composer.project.add")}</span>
      </DropdownMenu.Item>
    </DropdownMenu.Content>
  );
}

export function ComposerProjectTray() {
  const t = useT();
  const workIndex = useWorkIndex();
  const actions = useWorkIndexActions();
  if (workIndex.activeSessionId) return null;

  return (
    // `attached` is the decision, not the width: the tray tucks under the composer inset from
    // its edges, which a jsdom test can only ever check by reading back a class name.
    <AgentComposerTopTraySurface
      data-tray="attached"
      className={stylex.props(pl.traySurface).className}
    >
      <div data-slot="project-selector-tray" {...stylex.props(pl.tray)}>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger
            render={
              <Button
                type="button"
                size="sm"
                variant="ghost"
                press="none"
                disabled={!actions.canCreateSessionInFolder}
                aria-label={t("composer.project.choose")}
                title={t("composer.project.tooltip")}
                className={stylex.props(vocab.min).className}
              >
                <Icon name="folder" size="sm" className={stylex.props(vocab.hold).className} />
                <span {...stylex.props(pl.chooseLabel, vocab.truncate)}>
                  {t("composer.project.choose")}
                </span>
              </Button>
            }
          />
          <ProjectMenuContent
            groups={workIndex.groups}
            activeCwd={workIndex.activeCwd}
            loading={workIndex.isLoading}
            canCreate={actions.canCreateSessionInFolder}
            onSelect={actions.startSessionInFolder}
            onAdd={actions.chooseSessionFolder}
            align="start"
          />
        </DropdownMenu.Root>
      </div>
    </AgentComposerTopTraySurface>
  );
}

function ProjectNameTrigger({
  projectName,
  children,
}: {
  projectName: string;
  children?: ReactNode;
}) {
  const t = useT();
  return (
    <DropdownMenu.Trigger
      render={
        <Button
          type="button"
          variant="link"
          aria-label={t("composer.project.change", { project: projectName })}
          className={stylex.props(pl.full).className}
        >
          {children}
        </Button>
      }
    />
  );
}

export function EmptyChatHeading() {
  const t = useT();
  const workIndex = useWorkIndex();
  const actions = useWorkIndexActions();
  const activeCwd = workIndex.activeCwd;

  if (!workIndex.activeSessionId || !activeCwd) {
    return <>{t("welcome.title")}</>;
  }

  const projectName =
    workIndex.groups?.find(({ project }) => project.id === activeCwd)?.project.name ??
    basename(activeCwd);

  return (
    <DropdownMenu.Root>
      <Trans
        i18nKey="welcome.projectTitle"
        values={{ project: projectName }}
        components={{ projectSelect: <ProjectNameTrigger projectName={projectName} /> }}
      />
      <ProjectMenuContent
        groups={workIndex.groups}
        activeCwd={activeCwd}
        loading={workIndex.isLoading}
        canCreate={actions.canCreateSessionInFolder}
        onSelect={actions.startSessionInFolder}
        onAdd={actions.chooseSessionFolder}
        align="center"
      />
    </DropdownMenu.Root>
  );
}
