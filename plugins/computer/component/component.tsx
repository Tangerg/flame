import { METHODS } from "../contracts/methods";
import { useMemo, useRef, useState } from "react";
import { type ComponentContext, useTranslator } from "@flame/component-sdk";
import { handoffFocus, Panel, PanelHeader, PanelSection } from "@flame/ui";
import { LIMITS } from "../contracts/limits";
import { ApprovalForm } from "./approval";
import { useComputer } from "./useComputer";
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";
import { ConsoleControls, ConsoleFeedback } from "./controls";
import { HostDetails, LeaseForm, SessionDetails } from "./host";
import { ManualExecution } from "./execute";
import { ConsoleHistory } from "./history";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

function Console({
  context: initialContext,
  locale,
}: {
  context: ComponentContext;
  locale: string;
}) {
  const [context] = useState(initialContext);
  const t = useTranslator(CATALOGS, locale);
  const state = useComputer(context);
  const [lease, setLease] = useState(String(LIMITS.defaultLeaseSeconds));
  const [leaseError, setLeaseError] = useState(false);
  const editor = useRef<HTMLTextAreaElement>(null);
  const refresh = useRef<HTMLButtonElement>(null);
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
      new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "medium",
      }),
    [locale],
  );

  function open() {
    const leaseSeconds = Number(lease);
    if (
      !Number.isInteger(leaseSeconds) ||
      leaseSeconds < LIMITS.minLeaseSeconds ||
      leaseSeconds > LIMITS.maxLeaseSeconds
    ) {
      setLeaseError(true);
      return;
    }
    setLeaseError(false);
    void state.submit(METHODS.open, { leaseSeconds });
  }

  return (
    <Panel
      label={t("panel.title")}
      header={<PanelHeader title={t("panel.title")} />}
      controls={
        <ConsoleControls
          state={state}
          session={session}
          activeSession={active ? session : undefined}
          refresh={refresh}
          t={t}
        />
      }
    >
      <ConsoleFeedback state={state} t={t} />
      {state.snapshot && (
        <HostDetails snapshot={state.snapshot} locale={locale} t={t} />
      )}
      {session && (
        <SessionDetails
          session={session}
          active={active}
          dateTime={dateTime}
          t={t}
        />
      )}
      {!active && state.snapshot?.configured && (
        <LeaseForm
          value={lease}
          invalid={leaseError}
          disabled={!currentRead || state.busy(METHODS.open)}
          onChange={(value) => {
            setLease(value);
            setLeaseError(false);
          }}
          onOpen={open}
          t={t}
        />
      )}
      {session?.approval && (
        <PanelSection title={t("approval.title")} data-focus-unit="">
          <ApprovalForm
            key={session.approval.id}
            approval={session.approval}
            pending={state.busy(METHODS.reply)}
            respond={(input) => state.submit(METHODS.reply, input)}
            beforeRespond={(source) => {
              if (!handoffFocus(source, editor.current))
                handoffFocus(source, refresh.current);
            }}
            t={t}
          />
        </PanelSection>
      )}
      {active && (
        <ManualExecution
          surface={state.surface}
          ready={ready}
          pending={state.busy(METHODS.execute)}
          execute={(input) => state.submit(METHODS.execute, input)}
          editor={editor}
          fallbackFocus={refresh}
          t={t}
        />
      )}
      <ConsoleHistory state={state} dateTime={dateTime} t={t} />
    </Panel>
  );
}

export function Computer({ context }: { context: ComponentContext }) {
  // Display wrappers do not retire captured observations or unsubmitted drafts.
  const { runtime, component, binding, track, signal } = context;
  const instance = useMemo(
    () => crypto.randomUUID(),
    [runtime, component, binding, track, signal],
  );
  return <Console key={instance} context={context} locale={context.locale} />;
}
