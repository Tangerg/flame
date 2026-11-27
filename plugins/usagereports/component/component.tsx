import { useMemo } from "react";
import { type PluginComponentProps } from "@flame/component-sdk";
import { isImeKey, Metadata, Panel, PanelHeader } from "@flame/ui";
import { reportTitles, type BoundContext } from "./model";
import { useReportArchive } from "./useReportArchive";
import {
  ArchiveCatalog,
  ArchiveControls,
  PendingCommands,
  RemovalDialog,
  SavedReport,
} from "./views";

export function UsageReports({ context, locale }: PluginComponentProps) {
  const bound = useMemo(() => {
    if (!context.binding)
      throw new Error("Saved usage reports require their declared backend");
    return { ...context, binding: context.binding };
  }, [context]);
  const identity = `${bound.binding.installationId}/${bound.binding.digest}/${bound.binding.contribution}`;
  return <Archive key={identity} context={bound} locale={locale} />;
}

function Archive({ context, locale }: { context: BoundContext; locale: string }) {
  const archive = useReportArchive(context, locale);
  const { t, fmt, locked, outcome, refs, catalog, report, removal } =
    archive;
  const opened = report.opened;
  const titleFor = reportTitles(
    [
      ...(catalog.read.accepted ?? []),
      opened?.header,
      removal.target,
      outcome?.kind === "savedArchived" ? outcome.header : undefined,
    ],
    t,
    fmt,
  );
  return (
    <Panel
      ref={refs.panel}
      label={t("panel.title")}
      onKeyDown={(event) => {
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          isImeKey(event.nativeEvent) ||
          !opened ||
          removal.target ||
          !event.currentTarget.contains(event.target as Node)
        )
          return;
        event.preventDefault();
        event.stopPropagation();
        if (!event.repeat) report.back();
      }}
      header={<PanelHeader title={t("panel.title")} />}
      controls={<ArchiveControls archive={archive} titleFor={titleFor} />}
    >
      {outcome && (
        <Metadata role="status">
          {outcome.kind === "savedArchived"
            ? t("message.savedArchived", {
                title: titleFor(outcome.header),
              })
            : "error" in outcome
              ? t(`message.${outcome.kind}`, { error: outcome.error })
              : t(`message.${outcome.kind}`)}
        </Metadata>
      )}
      <PendingCommands archive={archive} />
      {opened && (
        <SavedReport
          opened={opened}
          removeButton={refs.removeButton}
          pending={locked}
          remove={removal.request}
          t={t}
          fmt={fmt}
        />
      )}
      <div hidden={Boolean(opened)}>
        <ArchiveCatalog
          archive={catalog.read}
          t={t}
          fmt={fmt}
          titleFor={titleFor}
          onOpen={catalog.open}
        />
      </div>
      <RemovalDialog archive={archive} titleFor={titleFor} />
    </Panel>
  );
}
