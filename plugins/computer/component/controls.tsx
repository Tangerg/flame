import { METHODS } from "../contracts/methods";
import { type RefObject } from "react";
import { type Translate } from "@flame/component-sdk";
import {
  Button,
  Callout,
  Metadata,
  PanelSection,
  RecordBody,
  RecordGroup,
  Toolbar,
} from "@flame/ui";
import type { Snapshot } from "./types";
import { useComputer } from "./useComputer";

export function commandLabel(method: string, t: Translate) {
  switch (method) {
    case METHODS.open:
      return t("open.action");
    case METHODS.close:
      return t("close.action");
    case METHODS.stop:
      return t("stop.action");
    case METHODS.reply:
      return t("approval.actions");
    case METHODS.execute:
      return t("execute.action");
    default:
      return method;
  }
}

export type ComputerView = ReturnType<typeof useComputer>;

export function ConsoleControls({
  state,
  session,
  activeSession,
  refresh,
  t,
}: {
  state: Pick<
    ComputerView,
    "read" | "watchError" | "refresh" | "reconnect" | "busy" | "submit"
  >;
  session: Snapshot["session"];
  activeSession: Snapshot["session"];
  refresh: RefObject<HTMLButtonElement | null>;
  t: Translate;
}) {
  return (
    <Toolbar label={t("controls.label")}>
      <Button
        ref={refresh}
        disabled={state.read?.loading}
        onClick={() => void state.refresh()}
      >
        {t("refresh")}
      </Button>
      {state.watchError && (
        <Button onClick={state.reconnect}>{t("watch.reconnect")}</Button>
      )}
      {activeSession && (
        <Button
          disabled={
            state.busy(METHODS.close) || activeSession.state === "stopping"
          }
          onClick={() =>
            void state.submit(METHODS.close, { sessionId: activeSession.id })
          }
        >
          {t("close.action")}
        </Button>
      )}
      {session?.operation && (
        <Button
          variant="primary"
          disabled={state.busy(METHODS.stop) || session.state === "stopping"}
          onClick={() =>
            void state.submit(METHODS.stop, {
              sessionId: session.id,
              operationId: session.operation!.id,
            })
          }
        >
          {t("stop.action")}
        </Button>
      )}
    </Toolbar>
  );
}

export function ConsoleFeedback({
  state,
  t,
}: {
  state: Pick<
    ComputerView,
    "read" | "watchError" | "notice" | "commands" | "recover"
  >;
  t: Translate;
}) {
  return (
    <>
      {state.read?.loading && <p role="status">{t("reading")}</p>}
      {state.read?.error && (
        <div role="alert">
          <Metadata>{t("read.failed")}</Metadata>
          <RecordBody>{state.read.error}</RecordBody>
        </div>
      )}
      {state.watchError && (
        <Callout kind="warning">
          <RecordBody>{t("watch.failed")}</RecordBody>
          {state.watchError.kind === "failed" && (
            <RecordBody>{state.watchError.message}</RecordBody>
          )}
        </Callout>
      )}
      {state.notice && (
        <p role={state.notice.kind === "error" ? "alert" : "status"}>
          {state.notice.kind === "error"
            ? state.notice.text
            : t("command.accepted", {
                action: commandLabel(state.notice.method, t),
              })}
        </p>
      )}
      {state.commands.length > 0 && (
        <PanelSection title={t("command.title")}>
          {state.commands.map((command) => (
            <RecordGroup key={command.id} aria-label={t("command.original")}>
              <Metadata>{commandLabel(command.method, t)}</Metadata>
              <RecordBody>{command.id}</RecordBody>
              <p role={command.status === "unknown" ? "alert" : "status"}>
                {t(
                  command.status === "unknown"
                    ? "command.unknown"
                    : "command.running",
                )}
              </p>
              {command.error && <RecordBody>{command.error}</RecordBody>}
              {command.status === "unknown" && command.recover && (
                <Button onClick={() => state.recover(command)}>
                  {t("command.recover")}
                </Button>
              )}
            </RecordGroup>
          ))}
        </PanelSection>
      )}
    </>
  );
}
