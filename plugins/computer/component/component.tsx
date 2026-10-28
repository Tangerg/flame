import { useEffect, useMemo, useRef, useState } from "react";
import {
  createTranslator,
  failureText,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import { parseJSON } from "@qimu/sdk/json";
import {
  Badge,
  Button,
  Callout,
  Disclosure,
  EmptyState,
  Facts,
  FormField,
  FormRow,
  Metadata,
  Panel,
  PanelHeader,
  PanelSection,
  RecordBody,
  RecordGroup,
  Select,
  TextArea,
  TextField,
  Toolbar,
} from "@flame/ui";
import type { ExecuteInput, Surface } from "./types";
import { LIMITS } from "../contracts/limits";
import { ApprovalForm } from "./approval";
import { Outcome, Source } from "./result";
import { methods, useComputer } from "./useComputer";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

function commandLabel(method: string, t: Translate) {
  switch (method) {
    case methods.open:
      return t("open.action");
    case methods.close:
      return t("close.action");
    case methods.stop:
      return t("stop.action");
    case methods.reply:
      return t("approval.actions");
    case methods.execute:
      return t("execute.action");
    default:
      return method;
  }
}

function ExecuteForm({
  surface,
  ready,
  pending,
  execute,
  t,
}: {
  surface: Surface;
  ready: boolean;
  pending: boolean;
  execute(input: ExecuteInput): Promise<boolean>;
  t: Translate;
}) {
  const [tool, setTool] = useState<ExecuteInput["tool"]>("js");
  const [argumentsJSON, setArgumentsJSON] = useState("{}");
  const [review, setReview] = useState<ExecuteInput>();
  const [error, setError] = useState<string>();
  const editor = useRef<HTMLTextAreaElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (review) cancel.current?.focus();
  }, [review]);
  const current =
    review &&
    review.sessionId === surface.sessionId &&
    review.workerId === surface.workerId &&
    review.surfaceDigest === surface.surfaceDigest;

  function prepare() {
    if (!ready || pending) return;
    setError(undefined);
    try {
      const value = parseJSON(argumentsJSON);
      if (value === null || typeof value !== "object" || Array.isArray(value))
        throw new Error(t("execute.object"));
      setReview({
        sessionId: surface.sessionId,
        workerId: surface.workerId,
        surfaceDigest: surface.surfaceDigest,
        tool,
        argumentsJSON,
      });
    } catch (cause) {
      setError(failureText(cause));
    }
  }

  return (
    <RecordGroup>
      <Metadata>{t("execute.help")}</Metadata>
      {error && (
        <div role="alert">
          <RecordBody>{error}</RecordBody>
        </div>
      )}
      {review ? (
        <RecordGroup role="region" aria-label={t("execute.review")}>
          <Callout kind="warning">
            {t(
              review.tool === "js_reset"
                ? "execute.resetWarning"
                : "execute.warning",
            )}
          </Callout>
          <Facts
            values={[
              [t("session.id"), review.sessionId],
              [t("session.worker"), review.workerId],
              [t("execute.tool"), review.tool],
              [t("session.surface"), review.surfaceDigest],
            ]}
          />
          <Source value={review.argumentsJSON} label={t("execute.arguments")} />
          {!current && <Callout kind="warning">{t("execute.stale")}</Callout>}
          <Toolbar label={t("execute.actions")}>
            <Button
              ref={cancel}
              disabled={pending}
              onClick={() => {
                setReview(undefined);
                requestAnimationFrame(() => editor.current?.focus());
              }}
            >
              {t("execute.back")}
            </Button>
            <Button
              variant="primary"
              disabled={!ready || pending || !current}
              onClick={() => {
                const captured = review;
                setReview(undefined);
                void execute(captured);
              }}
            >
              {t("execute.confirm")}
            </Button>
          </Toolbar>
        </RecordGroup>
      ) : (
        <RecordGroup>
          <Select
            label={t("execute.tool")}
            value={tool}
            options={[
              { value: "js", label: "js" },
              { value: "js_reset", label: "js_reset" },
            ]}
            disabled={!ready || pending}
            onValueChange={(value) => {
              setTool(value as ExecuteInput["tool"]);
              setArgumentsJSON("{}");
            }}
          />
          <FormField label={t("execute.arguments")}>
            <TextArea
              ref={editor}
              font="mono"
              value={argumentsJSON}
              onChange={(event) => setArgumentsJSON(event.target.value)}
              rows={8}
              spellCheck={false}
              maxLength={LIMITS.maxInputBytes}
              disabled={!ready || pending}
            />
          </FormField>
          <Button disabled={!ready || pending} onClick={prepare}>
            {t("execute.review")}
          </Button>
        </RecordGroup>
      )}
      <Disclosure title={t("context.instructions")}>
        <Source
          value={surface.instructions}
          label={t("context.instructions")}
        />
      </Disclosure>
      <Disclosure title={t("context.tools")}>
        <Source value={surface.toolsJSON} label={t("context.tools")} />
      </Disclosure>
      <Disclosure title={t("context.server")}>
        <Source value={surface.serverJSON} label={t("context.server")} />
      </Disclosure>
    </RecordGroup>
  );
}

export function Computer({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const state = useComputer(context);
  const [lease, setLease] = useState(String(LIMITS.defaultLeaseSeconds));
  const [leaseError, setLeaseError] = useState(false);
  const session = state.snapshot?.session;
  const active =
    session !== undefined &&
    session.state !== "closed" &&
    session.state !== "faulted";
  const currentRead =
    state.read !== undefined &&
    !state.read.loading &&
    !state.read.error &&
    !state.watchError;
  const ready = currentRead && session?.state === "ready";
  const dateTime = useMemo(
    () =>
      new Intl.DateTimeFormat(context.locale, {
        dateStyle: "medium",
        timeStyle: "medium",
      }),
    [context.locale],
  );

  function open() {
    const leaseSeconds = Number(lease);
    if (
      !Number.isInteger(leaseSeconds) ||
      leaseSeconds < 60 ||
      leaseSeconds > LIMITS.maxLeaseSeconds
    ) {
      setLeaseError(true);
      return;
    }
    setLeaseError(false);
    void state.submit(methods.open, { leaseSeconds });
  }

  return (
    <Panel
      label={t("panel.title")}
      header={
        <PanelHeader
          title={t("panel.title")}
          description={t("panel.description")}
        />
      }
      controls={
        <Toolbar label={t("controls.label")}>
          <Button
            disabled={state.read?.loading}
            onClick={() => void state.refresh()}
          >
            {t("refresh")}
          </Button>
          {state.watchError && (
            <Button onClick={state.reconnect}>{t("watch.reconnect")}</Button>
          )}
          {active && (
            <Button
              disabled={
                state.busy(methods.close) || session.state === "stopping"
              }
              onClick={() =>
                void state.submit(methods.close, { sessionId: session.id })
              }
            >
              {t("close.action")}
            </Button>
          )}
          {session?.operation && (
            <Button
              variant="primary"
              disabled={
                state.busy(methods.stop) || session.state === "stopping"
              }
              onClick={() =>
                void state.submit(methods.stop, {
                  sessionId: session.id,
                  operationId: session.operation!.id,
                })
              }
            >
              {t("stop.action")}
            </Button>
          )}
        </Toolbar>
      }
    >
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
      {state.snapshot && (
        <PanelSection title={t("host.title")}>
          <Facts
            values={[
              [t("host.name"), state.snapshot.host.hostname],
              [
                t("host.platform"),
                `${state.snapshot.host.platform} / ${state.snapshot.host.arch}`,
              ],
              [
                t("host.configuration"),
                t(
                  state.snapshot.configured
                    ? "host.configured"
                    : "host.missing",
                ),
              ],
            ]}
          />
          <Metadata>{t("host.scope")}</Metadata>
          {!state.snapshot.configured && (
            <Callout kind="warning">{t("host.setup")}</Callout>
          )}
        </PanelSection>
      )}
      {session && (
        <PanelSection
          title={t("session.title")}
          actions={
            <Badge
              tone={
                session.state === "faulted"
                  ? "negative"
                  : session.state === "ready"
                    ? "success"
                    : "neutral"
              }
            >
              {t(`session.${session.state}`)}
            </Badge>
          }
        >
          <Facts
            values={[
              [t("session.id"), session.id],
              [t("session.worker"), session.workerId],
              [
                t("session.opened"),
                dateTime.format(new Date(session.openedAt)),
              ],
              [
                t("session.expires"),
                dateTime.format(new Date(session.expiresAt)),
              ],
              [t("session.surface"), session.surfaceDigest],
              [
                t("session.reason"),
                session.reason ? t(`reason.${session.reason}`) : undefined,
              ],
            ]}
          />
          {session.operation && (
            <Callout kind="note">
              <Facts
                values={[
                  [t("operation.id"), session.operation.id],
                  [t("execute.tool"), session.operation.tool],
                  [
                    t("operation.started"),
                    dateTime.format(new Date(session.operation.startedAt)),
                  ],
                ]}
              />
            </Callout>
          )}
          {active && <Metadata>{t("close.help")}</Metadata>}
        </PanelSection>
      )}
      {!active && state.snapshot?.configured && (
        <PanelSection title={t("open.title")} description={t("open.help")}>
          <FormRow>
            <FormField label={t("open.lease")}>
              <TextField
                value={lease}
                inputMode="numeric"
                invalid={leaseError}
                onChange={(event) => {
                  setLease(event.target.value);
                  setLeaseError(false);
                }}
                disabled={!currentRead || state.busy(methods.open)}
              />
            </FormField>
          </FormRow>
          {leaseError && (
            <p role="alert">
              {t("open.invalid", { maximum: String(LIMITS.maxLeaseSeconds) })}
            </p>
          )}
          <Button
            variant="primary"
            disabled={!currentRead || state.busy(methods.open)}
            onClick={open}
          >
            {t("open.action")}
          </Button>
        </PanelSection>
      )}
      {session?.approval && (
        <PanelSection title={t("approval.title")} data-focus-unit="">
          <ApprovalForm
            key={session.approval.id}
            approval={session.approval}
            pending={state.busy(methods.reply)}
            respond={(input) => state.submit(methods.reply, input)}
            t={t}
          />
        </PanelSection>
      )}
      {active && (
        <PanelSection title={t("execute.title")}>
          {state.surface?.loading && (
            <p role="status">{t("context.reading")}</p>
          )}
          {state.surface?.error && (
            <div role="alert">
              <RecordBody>{state.surface.error}</RecordBody>
            </div>
          )}
          {state.surface?.value && (
            <ExecuteForm
              surface={state.surface.value}
              ready={ready}
              pending={state.busy(methods.execute)}
              execute={(input) => state.submit(methods.execute, input)}
              t={t}
            />
          )}
        </PanelSection>
      )}
      <PanelSection
        title={t("history.title")}
        description={t("history.help")}
        actions={
          <Button
            disabled={state.history?.loading}
            onClick={() => void state.loadHistory()}
          >
            {t("history.refresh")}
          </Button>
        }
      >
        {state.history?.loading && <p role="status">{t("history.reading")}</p>}
        {state.history?.error && (
          <div role="alert">
            <RecordBody>{state.history.error}</RecordBody>
          </div>
        )}
        {state.history?.value && state.history.value.entries.length === 0 && (
          <EmptyState title={t("history.empty")} />
        )}
        {state.history?.value?.entries.map((entry) => (
          <RecordGroup key={entry.id}>
            <Toolbar label={entry.id}>
              <Button onClick={() => void state.inspect(entry.id)}>
                {entry.id}
              </Button>
              <Badge tone={entry.settled ? "neutral" : "warning"}>
                {t(entry.settled ? "history.settled" : "history.unknown")}
              </Badge>
            </Toolbar>
            <Metadata>
              {commandLabel(entry.method, t)} ·{" "}
              {dateTime.format(new Date(entry.admittedAt))}
            </Metadata>
          </RecordGroup>
        ))}
        <Toolbar label={t("history.pages")}>
          <Button
            disabled={!state.hasPrevious || state.history?.loading}
            onClick={() => void state.previousPage()}
          >
            {t("history.previous")}
          </Button>
          <Button
            disabled={!state.history?.value?.next || state.history.loading}
            onClick={() => void state.nextPage()}
          >
            {t("history.next")}
          </Button>
        </Toolbar>
      </PanelSection>
      {state.result && (
        <PanelSection
          title={t("result.title")}
          actions={
            state.result.value && (
              <Button
                disabled={state.result.loading}
                onClick={() => void state.inspect(state.result!.value!.id)}
              >
                {t("refresh")}
              </Button>
            )
          }
        >
          {state.result.loading && <p role="status">{t("history.reading")}</p>}
          {state.result.error && (
            <div role="alert">
              <RecordBody>{state.result.error}</RecordBody>
            </div>
          )}
          {state.result.value && <Outcome record={state.result.value} t={t} />}
        </PanelSection>
      )}
    </Panel>
  );
}
