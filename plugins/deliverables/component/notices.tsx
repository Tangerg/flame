import { type Translate } from "@flame/component-sdk";
import {
  Button,
  Callout,
  Metadata,
  PanelSection,
  RecordGroup,
  Toolbar,
} from "@flame/ui";
import { METHODS } from "../contracts/methods";
import { Source } from "./provenance";
import { type OpenForReading } from "./archive";
import type { CommandMethod } from "./types";
import type { DeliverablesState } from "./useDeliverables";

// The label of the control that starts each command.
const COMMAND_LABELS: Record<CommandMethod, string> = {
  [METHODS.create]: "author.create",
  [METHODS.edit]: "edit.apply",
  [METHODS.upload]: "import.submit",
  [METHODS.append]: "import.submit",
  [METHODS.import]: "import.submit",
  [METHODS.discard]: "import.discard",
  [METHODS.render]: "preview.render",
  [METHODS.recalculate]: "preview.recalculate",
  [METHODS.restore]: "history.restore",
  [METHODS.comment]: "comments.add",
  [METHODS.reply]: "comments.reply",
  [METHODS.resolve]: "comments.resolve",
  [METHODS.reopen]: "comments.reopen",
  [METHODS.review]: "review.submit",
  [METHODS.export]: "export.prepare",
  [METHODS.remove]: "remove.action",
};

const commandLabel = (method: CommandMethod, t: Translate) =>
  t(COMMAND_LABELS[method]);

export function PendingCommands({
  state,
  t,
}: {
  state: DeliverablesState;
  t: Translate;
}) {
  if (!state.commands.length) return null;
  return (
    <PanelSection title={t("command.title")} description={t("command.help")}>
      {state.commands.map((command) => (
        <RecordGroup key={command.id}>
          <Metadata role="status">
            {commandLabel(command.method, t)} ·{" "}
            {t(
              command.status === "running"
                ? "command.running"
                : "command.unknown",
            )}
          </Metadata>
          {command.error && <Callout kind="warning">{command.error}</Callout>}
          <Toolbar label={t("command.actions")}>
            <Button
              disabled={command.status !== "unknown" || !command.recover}
              onClick={() => state.recover(command)}
            >
              {t("command.recover")}
            </Button>
            <Button
              disabled={command.status !== "unknown" || !command.retire}
              onClick={() => {
                void state.retire(command);
              }}
            >
              {t("command.retire")}
            </Button>
          </Toolbar>
          <Source label={t("command.identity")} value={command.id} />
        </RecordGroup>
      ))}
    </PanelSection>
  );
}

export function Notices({
  state,
  onOpen,
  t,
}: {
  state: DeliverablesState;
  onOpen: OpenForReading;
  t: Translate;
}) {
  const notice = state.notice;
  return (
    <RecordGroup aria-live="polite">
      {notice?.kind === "error" && (
        <Callout kind="warning">{notice.text}</Callout>
      )}
      {notice?.kind === "save" && (
        <Metadata>
          {t(`save.${notice.result}`, { filename: notice.filename })}
        </Metadata>
      )}
      {notice?.kind === "command" && (
        <>
          <Metadata>
            {t("command.complete", { action: commandLabel(notice.method, t) })}
          </Metadata>
          {notice.asset && (
            <Button
              disabled={!!state.transfer}
              onClick={() => {
                if (notice.asset) void state.save(notice.asset);
              }}
            >
              {t("export.savePrepared")}
            </Button>
          )}
          {notice.revision && (
            <Button
              onClick={(event) => {
                if (notice.revision)
                  onOpen(
                    {
                      id: notice.revision.documentId,
                      revision: notice.revision.id,
                    },
                    event.currentTarget,
                  );
              }}
            >
              {t("command.openRevision")}
            </Button>
          )}
        </>
      )}
      {state.transfer && (
        <Toolbar label={t("transfer.title")}>
          <Metadata>
            {state.transfer.filename} ·{" "}
            {t("transfer.progress", {
              received: state.transfer.received,
              total: state.transfer.total,
            })}
          </Metadata>
          <Button
            disabled={state.transfer.received >= state.transfer.total}
            onClick={state.cancelTransfer}
          >
            {t("transfer.cancel")}
          </Button>
        </Toolbar>
      )}
    </RecordGroup>
  );
}
