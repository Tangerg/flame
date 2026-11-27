import { useState, useTransition, type ReactNode } from "react";
import { failureText, type ComponentContext } from "@flame/component-sdk";
import {
  Badge,
  Button,
  Facts,
  ListRow,
  Metadata,
  ModalDialog,
  ActionMenu,
  Disclosure,
  RecordBody,
  RecordExcerpt,
  RecordHeading,
} from "@flame/ui";
import { enabled } from "./useSchedules";
import { type Schedule } from "./types";
import { type ScheduleState } from "./useSchedules";
import { METHODS } from "./methods";

export function ScheduleDeletion({
  t,
  item,
  feedback,
  busy,
  deleteCancel,
  resolveOpener,
  cancelTask,
  deleteConfirmed,
}: Pick<
  ScheduleState,
  | "t"
  | "busy"
  | "deleteCancel"
  | "resolveOpener"
  | "cancelTask"
  | "deleteConfirmed"
> & { item: Schedule; feedback: ReactNode }) {
  return (
    <ModalDialog
      open
      role="alertdialog"
      title={t("delete.title", { name: item.title || item.id })}
      description={t("delete.description")}
      closeLabel={t("delete.close")}
      size="sm"
      initialFocus={deleteCancel}
      finalFocus={resolveOpener}
      dismissible={!busy}
      onOpenChange={(open) => {
        if (!open) cancelTask();
      }}
      auxiliaryAction={feedback}
      footer={
        <>
          <Button
            ref={deleteCancel}
            variant="ghost"
            disabled={busy}
            onClick={cancelTask}
          >
            {t("cancel")}
          </Button>
          <Button
            variant="tonal"
            tone="negative"
            disabled={busy}
            onClick={deleteConfirmed}
          >
            {t("delete.confirm")}
          </Button>
        </>
      }
    />
  );
}

export function ScheduleRow({
  t,
  dateTime,
  item,
  disabled,
  command,
  feedbackElement,
  runNow,
  editSchedule,
  toggleEnabled,
  confirmDeletion,
}: Pick<
  ScheduleState,
  | "t"
  | "dateTime"
  | "disabled"
  | "command"
  | "feedbackElement"
  | "runNow"
  | "editSchedule"
  | "toggleEnabled"
  | "confirmDeletion"
> & { item: Schedule }) {
  const title = item.title || t("untitled");
  const { provider, model, reasoningEffort } = item.modelSelection;
  return (
    <ListRow>
      <RecordHeading
        title={title}
        description={
          // Instructions read in full under the schedule's details.
          item.instructions && (
            <RecordExcerpt>{item.instructions}</RecordExcerpt>
          )
        }
        actions={
          <ActionMenu
            label={t("actions.more", { name: title })}
            disabled={disabled}
            finalFocus={
              command?.command.method === METHODS.runNow
                ? feedbackElement
                : undefined
            }
            items={[
              { label: t("action.runNow"), onSelect: () => runNow(item) },
              {
                label: t("action.edit"),
                onSelect: (opener) => editSchedule(item, opener),
              },
              {
                label: enabled(item) ? t("action.disable") : t("action.enable"),
                onSelect: () => toggleEnabled(item),
              },
              {
                label: t("action.delete"),
                tone: "negative",
                onSelect: (opener) => confirmDeletion(item, opener),
              },
            ]}
          />
        }
        // Enabled is the resting state; only a paused schedule needs a word.
        status={
          enabled(item) ? undefined : <Badge>{t("status.disabled")}</Badge>
        }
      />
      {/* A disabled schedule says so in its status; "not scheduled" would only repeat it. */}
      {(enabled(item) || item.nextRunAt !== undefined) && (
        <Metadata>
          {t("nextFiring.label")} ·{" "}
          {item.nextRunAt !== undefined ? (
            <time dateTime={item.nextRunAt}>
              {dateTime.format(new Date(item.nextRunAt))}
            </time>
          ) : (
            t("nextFiring.none")
          )}
        </Metadata>
      )}
      <Disclosure title={t("details.title")}>
        {item.instructions && <RecordBody>{item.instructions}</RecordBody>}
        <Facts
          values={[
            [t("fact.cron"), item.cron],
            [t("fact.timeZone"), item.timeZone],
            [t("fact.schedule"), item.id],
            [t("fact.revision"), String(item.revision)],
            [t("fact.workspace"), item.cwd || t("runtimeDefault")],
            [
              t("field.model"),
              provider && model ? `${provider}/${model}` : t("runtimeDefault"),
            ],
            [t("field.reasoning"), reasoningEffort || undefined],
            [
              t("fact.lastFiring"),
              item.lastRunAt !== undefined
                ? dateTime.format(new Date(item.lastRunAt))
                : t("fact.neverFired"),
            ],
            [t("fact.created"), dateTime.format(new Date(item.createdAt))],
          ]}
        />
      </Disclosure>
    </ListRow>
  );
}

export function ScheduleFeedback({
  t,
  notice,
  feedbackElement,
  command,
  reconcile,
  openSession,
  signal,
}: Pick<
  ScheduleState,
  "t" | "notice" | "feedbackElement" | "command" | "reconcile"
> & { openSession: ComponentContext["openSession"]; signal: AbortSignal }) {
  return (
    <section
      ref={feedbackElement}
      tabIndex={-1}
      data-focus-inset=""
      aria-label={t("command.status.label")}
    >
      {notice &&
        (typeof notice === "string" ? (
          <p role="status">{notice}</p>
        ) : (
          <>
            <p role="status">
              {t("notice.accepted", {
                run: notice.runId,
                session: notice.sessionId,
              })}
            </p>
            {openSession && (
              <OpenScheduledConversation
                key={notice.runId}
                sessionId={notice.sessionId}
                open={openSession}
                signal={signal}
                t={t}
              />
            )}
          </>
        ))}
      {command && (
        <>
          <p role={command.phase === "uncertain" ? "alert" : "status"}>
            {command.phase === "uncertain"
              ? t("command.uncertain")
              : t("command.progress", { label: command.command.label })}
          </p>
          {command.phase === "uncertain" && (
            <>
              <RecordBody>{command.error}</RecordBody>
              <Metadata>
                {t("command.original", {
                  key: command.command.promise.idempotencyKey,
                })}
              </Metadata>
              <Button variant="soft" onClick={reconcile}>
                {t("command.reconcile")}
              </Button>
            </>
          )}
        </>
      )}
    </section>
  );
}

function OpenScheduledConversation({
  sessionId,
  open,
  signal,
  t,
}: {
  sessionId: string;
  open: NonNullable<ComponentContext["openSession"]>;
  signal: AbortSignal;
  t: ScheduleState["t"];
}) {
  const [pending, startOpening] = useTransition();
  const [problem, setProblem] = useState<string>();
  return (
    <>
      <Button
        variant="soft"
        pending={pending}
        onClick={() => {
          setProblem(undefined);
          startOpening(async () => {
            try {
              if ((await open(sessionId)) === "missing")
                setProblem(t("notice.sessionMissing"));
            } catch (error) {
              if (!signal.aborted) setProblem(failureText(error));
            }
          });
        }}
      >
        {t("notice.openSession")}
      </Button>
      {problem && <p role="alert">{problem}</p>}
    </>
  );
}
