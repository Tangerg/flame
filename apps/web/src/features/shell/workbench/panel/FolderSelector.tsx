import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { basename } from "@/lib/path";
import { Trans, useT } from "@/lib/i18n/index";
import { useWorkIndex } from "@/features/navigation/public/workIndex";
import { useWorkIndexActions } from "@/features/navigation/public/workIndex";
import { type WorkGroup } from "@/features/navigation/public/workIndex";
import { DropdownMenu } from "@/ui/index";
import { Icon } from "@/ui/index";
import { Button, vocab } from "@flame/ui";
import { AgentComposerTopTraySurface } from "@/ui/agent/index";
import { face, space, type as typeStep } from "@flame/ui/tokens.stylex";
interface FolderMenuContentProps {
  groups: readonly WorkGroup[] | undefined;
  activeCwd: string | undefined;
  loading: boolean;
  canCreate: boolean;
  onSelect: (cwd: string) => void;
  onAdd: () => void;
  align: "start" | "center";
}

const pl = stylex.create({
  menu: { width: "min(320px, calc(100vw - 32px))" },
  tray: { display: "flex", minWidth: 0, alignItems: "center" },
  traySurface: {
    position: "relative",
    top: space.s1,
    zIndex: 0,
    marginInline: space.s3,
    marginBottom: "-18px",
    width: "calc(100% - 24px)",
    backgroundColor: "var(--app-composer-folder-tray-surface)",
    paddingInline: space.s1_5,
    paddingTop: space.s1_5,
    paddingBottom: "27px",
    backdropFilter: "none",
    WebkitBackdropFilter: "none",
  },
  chooseLabel: { maxWidth: "240px" },
  full: { maxWidth: "100%" },
  folderIdentity: {
    display: "grid",
    minWidth: 0,
    gap: space.s0_5,
    whiteSpace: "normal",
    overflowWrap: "anywhere",
  },
});

function FolderMenuContent({
  groups,
  activeCwd,
  loading,
  canCreate,
  onSelect,
  onAdd,
  align,
}: FolderMenuContentProps) {
  const t = useT();
  const nameCounts = new Map<string, number>();
  for (const { folder } of groups ?? [])
    nameCounts.set(folder.name, (nameCounts.get(folder.name) ?? 0) + 1);
  return (
    <DropdownMenu.Content
      side="top"
      align={align}
      sideOffset={8}
      className={stylex.props(pl.menu).className}
    >
      {loading && !groups ? (
        <DropdownMenu.Item disabled layout="glyph">
          <Icon name="folder" size="sm" className={stylex.props(vocab.faint).className} />
          <span>{t("common.loading")}</span>
        </DropdownMenu.Item>
      ) : (
        groups?.map(({ folder }) => (
          <DropdownMenu.Item
            key={folder.id}
            disabled={folder.cwdMissing || !canCreate}
            aria-current={folder.id === activeCwd ? "true" : undefined}
            onClick={() => {
              if (folder.id !== activeCwd) onSelect(folder.id);
            }}
            title={folder.cwdMissing ? t("folder.row.missing") : folder.id}
            layout="pick"
          >
            <Icon name="folder" size="sm" className={stylex.props(vocab.muted).className} />
            <span {...stylex.props(pl.folderIdentity)}>
              <span>{folder.name}</span>
              {nameCounts.get(folder.name)! > 1 && (
                <span dir="ltr" {...stylex.props(face.mono, typeStep.uiXs, vocab.muted)}>
                  {folder.id}
                </span>
              )}
            </span>
            {folder.id === activeCwd ? (
              <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
            ) : (
              <span aria-hidden />
            )}
          </DropdownMenu.Item>
        ))
      )}
      {((loading && !groups) || Boolean(groups?.length)) && <DropdownMenu.Separator />}
      <DropdownMenu.Item disabled={!canCreate} onClick={onAdd} layout="glyph">
        <Icon name="plus" size="sm" className={stylex.props(vocab.muted).className} />
        <span>{t("composer.folder.add")}</span>
      </DropdownMenu.Item>
    </DropdownMenu.Content>
  );
}

export function ComposerFolderTray() {
  const t = useT();
  const workIndex = useWorkIndex();
  const actions = useWorkIndexActions();
  if (workIndex.activeSessionId) return null;

  return (
    <AgentComposerTopTraySurface
      data-tray="attached"
      className={stylex.props(pl.traySurface).className}
    >
      <div data-slot="folder-selector-tray" {...stylex.props(pl.tray)}>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger
            disabled={!actions.canCreateSessionInFolder}
            render={
              <Button
                type="button"
                size="sm"
                variant="ghost"
                press="none"
                aria-label={t("composer.folder.choose")}
                className={stylex.props(vocab.min).className}
              >
                <Icon name="folder" size="sm" className={stylex.props(vocab.hold).className} />
                <span {...stylex.props(pl.chooseLabel, vocab.truncate)}>
                  {t("composer.folder.choose")}
                </span>
              </Button>
            }
          />
          <FolderMenuContent
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

function FolderNameTrigger({ folderName, children }: { folderName: string; children?: ReactNode }) {
  const t = useT();
  return (
    <DropdownMenu.Trigger
      render={
        <Button
          type="button"
          variant="link"
          aria-label={t("composer.folder.change", { folder: folderName })}
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

  const folderName =
    workIndex.groups?.find(({ folder }) => folder.id === activeCwd)?.folder.name ??
    basename(activeCwd);

  return (
    <DropdownMenu.Root>
      <Trans
        i18nKey="welcome.folderTitle"
        values={{ folder: folderName }}
        components={{ folderSelect: <FolderNameTrigger folderName={folderName} /> }}
      />
      <FolderMenuContent
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
