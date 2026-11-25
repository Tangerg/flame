import { type Translate } from "@flame/component-sdk";
import { Button, Metadata, RecordBody } from "@flame/ui";
import {
  type CommandIntent,
  type CommandState,
  type Notice,
  type SkillRecord,
  type SkillSnapshot,
} from "./types";
import { type SkillsState } from "./useSkills";

function commandLabel(intent: CommandIntent, t: Translate) {
  return t(`command.${intent.action}`, { name: intent.name });
}

function noticeText(notice: Notice, t: Translate) {
  switch (notice.kind) {
    case "completed":
      return t("notice.completed", { label: commandLabel(notice.intent, t) });
    case "revisionConflict":
      return t("notice.revisionConflict");
    case "failure":
      return notice.detail;
  }
}

export function SkillCommandFeedback({
  command,
  notice,
  reconcile,
  t,
}: Pick<SkillsState, "command" | "notice" | "reconcile" | "t">) {
  return (
    <>
      {notice && <p role="status">{noticeText(notice, t)}</p>}
      {command && (
        <section aria-label={t("command.status.label")}>
          <p role={command.phase === "uncertain" ? "alert" : "status"}>
            {command.phase === "uncertain"
              ? t("command.uncertain")
              : t("command.pending", {
                  label: commandLabel(command.command.intent, t),
                })}
          </p>
          {command.phase === "uncertain" && (
            <>
              <RecordBody>{command.error}</RecordBody>
              <Metadata>
                {t("command.original", {
                  key: command.command.promise.idempotencyKey,
                })}
              </Metadata>
              <Button onClick={reconcile}>{t("command.reconcile")}</Button>
            </>
          )}
        </section>
      )}
    </>
  );
}

export function SkillReadStatus({
  opened,
  read,
  snapshot,
  skillDocument,
  title,
  observationError,
  t,
}: Pick<
  SkillsState,
  | "opened"
  | "read"
  | "snapshot"
  | "skillDocument"
  | "title"
  | "observationError"
  | "t"
>) {
  return (
    <>
      {!opened && read.loading && <p role="status">{t("reading")}</p>}
      {!opened && read.error && (
        <p role="alert">
          {t("read.error", {
            catalog: title.toLocaleLowerCase(),
            error: read.error,
          })}
          {snapshot && ` ${t("read.retained")}`}
        </p>
      )}
      {opened?.loading && (
        <p role="status">{t("document.reading", { name: opened.name })}</p>
      )}
      {opened?.error && (
        <p role="alert">
          {skillDocument
            ? t("document.error.refresh", {
                name: opened.name,
                error: opened.error,
              })
            : t("document.error.read", {
                name: opened.name,
                error: opened.error,
              })}
        </p>
      )}
      {opened?.error && skillDocument && (
        <Metadata role="status">
          {t("document.retained", { name: opened.name })}
        </Metadata>
      )}
      {observationError && (
        <p role="alert">
          {t("observation.error", { error: observationError })}
        </p>
      )}
    </>
  );
}

// Archive and restore feedback sits on the row it concerns while that row is listed.
export function feedbackRowName(
  command: CommandState | undefined,
  notice: Notice | undefined,
  snapshot: SkillSnapshot | undefined,
  matching: readonly SkillRecord[],
): string | undefined {
  const intent = command?.command.intent ?? notice?.intent;
  if (intent?.action !== "archive" && intent?.action !== "restore") return;
  if (snapshot?.type !== "library") return;
  return matching.some((item) => item.name === intent.name)
    ? intent.name
    : undefined;
}
