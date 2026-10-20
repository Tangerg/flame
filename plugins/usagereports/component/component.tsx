import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type Ref,
} from "react";
import { ArrowLeft } from "@flame/icons/react";
import {
  bindOperations,
  mutationSettlementIsUnknown,
  type ComponentContext,
} from "@flame/component-sdk";
import type { JSONNumber } from "@flame/runtime-client";
import {
  Button,
  Disclosure,
  EmptyState,
  Facts,
  Icon,
  ListRow,
  Metadata,
  ModalDialog,
  Panel,
  PanelHeader,
  PanelSection,
  PanelTitle,
  RecordGroup,
  RecordHeading,
  Select,
  Toolbar,
} from "@flame/ui";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";

const captureMethod = "flame.usage-reports/capture.v1";
const listMethod = "flame.usage-reports/list.v1";
const getMethod = "flame.usage-reports/get.v1";
const deleteMethod = "flame.usage-reports/delete.v1";
type Header = MethodOutputs[typeof listMethod][number];
type Report = MethodOutputs[typeof getMethod];
type Period = MethodInputs[typeof captureMethod]["period"];
type BoundContext = ComponentContext & {
  readonly binding: NonNullable<ComponentContext["binding"]>;
};
type ArchiveRead = { loading: boolean; accepted?: Header[]; error?: string };
type OpenedReport = {
  header: Header;
  loading: boolean;
  accepted?: Report;
  error?: string;
};
type ScrollPosition = { element: HTMLElement; top: number };
type ArchiveReturn = { opener?: HTMLElement; scroll: ScrollPosition[] };
const periods = {
  daily: { label: "Daily", window: "last 24 hours" },
  weekly: { label: "Weekly", window: "last 7 days" },
} satisfies Record<Period, { label: string; window: string }>;
const number = new Intl.NumberFormat("en-US");
const dollars = new Intl.NumberFormat("en-US", { maximumFractionDigits: 6 });
const format = (value: JSONNumber | undefined, formatter: Intl.NumberFormat) =>
  value === undefined
    ? "Not reported"
    : typeof value === "object"
      ? value.toString()
      : formatter.format(value);
const amount = (value?: JSONNumber) => format(value, dollars);
const count = (value?: JSONNumber) => format(value, number);
const failure = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const observed = (header: Header) =>
  new Date(header.observedAt).toLocaleString();
const lowerBound = (header: Header) =>
  header.since
    ? new Date(header.since).toLocaleString()
    : "Beginning of retained evidence";
const reportTitle = (header: Header) =>
  `${periods[header.period].label} report · ${observed(header)}`;

function scrollPositions(element: HTMLElement | null): ScrollPosition[] {
  const positions: ScrollPosition[] = [];
  for (
    let parent = element?.parentElement;
    parent;
    parent = parent.parentElement
  ) {
    if (/(auto|scroll)/.test(getComputedStyle(parent).overflowY))
      positions.push({ element: parent, top: parent.scrollTop });
  }
  return positions;
}

export function UsageReports({ context }: { context: ComponentContext }) {
  const bound = useMemo(() => {
    if (!context.binding)
      throw new Error("Saved usage reports require their declared backend");
    return { ...context, binding: context.binding };
  }, [context]);
  const identity = `${bound.binding.installationId}/${bound.binding.digest}/${bound.binding.contribution}`;
  return <Archive key={identity} context={bound} />;
}

function Archive({ context }: { context: BoundContext }) {
  const operations = useMemo(
    () =>
      bindOperations<MethodInputs, MethodOutputs>(context, CAPABILITY_JSON, {
        scope: "installation",
        reference: context.binding.installationId,
      }),
    [context],
  );
  const [archive, setArchive] = useState<ArchiveRead>({ loading: true });
  const [opened, setOpened] = useState<OpenedReport>();
  const [removing, setRemoving] = useState<Header>();
  const [period, setPeriod] = useState<Period>("daily");
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState<string>();
  const lifetime = useRef<AbortController>(undefined);
  const inspecting = useRef<AbortController>(undefined);
  const listing = useRef<AbortController>(undefined);
  const panel = useRef<HTMLElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const refreshButton = useRef<HTMLButtonElement>(null);
  const backButton = useRef<HTMLButtonElement>(null);
  const removeButton = useRef<HTMLButtonElement>(null);
  const cancelRemoval = useRef<HTMLButtonElement>(null);
  // Command outcomes settle after awaits; they decide against the committed
  // view rather than the view captured when the command started.
  const shown = useRef<OpenedReport>(undefined);
  const archiveReturn = useRef<ArchiveReturn>(undefined);
  const pendingFocus = useRef<{
    target: "report" | "archive";
    active: Element | null;
  }>(null);
  const unknown = [
    ...operations.pendingCommands(captureMethod),
    ...operations.pendingCommands(deleteMethod),
  ];

  const refresh = useCallback(
    async (owner: AbortSignal): Promise<Header[] | undefined> => {
      listing.current?.abort();
      const request = new AbortController();
      listing.current = request;
      const signal = AbortSignal.any([owner, request.signal]);
      setArchive((previous) => ({
        ...previous,
        loading: true,
        error: undefined,
      }));
      try {
        const result = await operations.query(listMethod, {}, signal);
        if (signal.aborted) return;
        setArchive({ loading: false, accepted: result });
        return result;
      } catch (error) {
        if (!signal.aborted)
          setArchive((previous) => ({
            ...previous,
            loading: false,
            error: failure(error),
          }));
      }
    },
    [operations],
  );

  useEffect(() => {
    const owner = new AbortController();
    lifetime.current = owner;
    void refresh(AbortSignal.any([owner.signal, context.signal]));
    return () => {
      owner.abort();
      inspecting.current?.abort();
      listing.current?.abort();
    };
  }, [context.signal, refresh]);

  useLayoutEffect(() => {
    shown.current = opened;
  }, [opened]);

  const openedId = opened?.header.id;
  useLayoutEffect(() => {
    const requested = pendingFocus.current;
    pendingFocus.current = null;
    if (!requested) return;
    const active = panel.current?.ownerDocument.activeElement;
    const focusIsUnclaimed =
      active === requested.active || active === active?.ownerDocument.body;
    if (requested.target === "report") {
      for (const position of scrollPositions(panel.current))
        position.element.scrollTop = 0;
      if (focusIsUnclaimed) heading.current?.focus();
      return;
    }
    const restore = archiveReturn.current;
    archiveReturn.current = undefined;
    for (const position of restore?.scroll ?? [])
      position.element.scrollTop = position.top;
    if (focusIsUnclaimed)
      (restore?.opener?.isConnected
        ? restore.opener
        : refreshButton.current
      )?.focus();
  }, [openedId]);

  function ownerSignal() {
    const owner = lifetime.current;
    if (!owner) throw new Error("Report component lifetime has not started");
    return AbortSignal.any([context.signal, owner.signal]);
  }

  function activeElement() {
    return panel.current?.ownerDocument.activeElement ?? null;
  }

  async function read(id: string) {
    inspecting.current?.abort();
    const request = new AbortController();
    inspecting.current = request;
    const signal = AbortSignal.any([ownerSignal(), request.signal]);
    setOpened((previous) =>
      previous?.header.id === id
        ? { ...previous, loading: true, error: undefined }
        : previous,
    );
    try {
      const accepted = await operations.query(getMethod, { id }, signal);
      if (!signal.aborted)
        setOpened((previous) =>
          previous?.header.id === id
            ? { ...previous, loading: false, accepted }
            : previous,
        );
    } catch (error) {
      if (!signal.aborted)
        setOpened((previous) =>
          previous?.header.id === id
            ? { ...previous, loading: false, error: failure(error) }
            : previous,
        );
    }
  }

  function open(
    header: Header,
    opener: HTMLElement | undefined,
    active: Element | null,
  ) {
    if (!shown.current)
      archiveReturn.current = {
        opener,
        scroll: scrollPositions(panel.current),
      };
    pendingFocus.current = { target: "report", active };
    setOpened({ header: structuredClone(header), loading: true });
    void read(header.id);
  }

  function leave(active: Element | null, returnToOpener: boolean) {
    inspecting.current?.abort();
    if (!returnToOpener && archiveReturn.current)
      archiveReturn.current.opener = undefined;
    pendingFocus.current = { target: "archive", active };
    setOpened(undefined);
  }

  async function observe(command: Promise<Header | boolean>, removed?: Header) {
    const signal = ownerSignal();
    const active = activeElement();
    setPending(true);
    setMessage(undefined);
    try {
      const result = await command;
      if (signal.aborted) return;
      if (typeof result !== "boolean") {
        const current = activeElement();
        // A later completion opens the saved report only while the user
        // remains where the command started.
        if (current === active || current === current?.ownerDocument.body) {
          setMessage("Core report saved.");
          open(result, undefined, active);
        } else
          setMessage(
            `Core report saved: ${reportTitle(result)}. It is listed in the archive.`,
          );
        void refresh(signal);
        return;
      }
      setMessage(
        removed
          ? "Saved report removed. Core accounting is unchanged."
          : "Original report removal confirmed. Core accounting is unchanged.",
      );
      const accepted = await refresh(signal);
      if (signal.aborted) return;
      // A recovered removal reports success without its original target. Only
      // the captured target or fresh canonical membership can retire the open
      // report; an unread archive keeps it.
      const current = shown.current;
      if (
        current &&
        (current.header.id === removed?.id ||
          (accepted && !accepted.some(({ id }) => id === current.header.id)))
      )
        leave(active, false);
    } catch (error) {
      if (!signal.aborted)
        setMessage(
          mutationSettlementIsUnknown(error)
            ? "Confirmation is unknown. Check the original outcome before submitting another archive change."
            : `The archive change failed: ${failure(error)}`,
        );
    } finally {
      if (!signal.aborted) {
        setPending(false);
        setRemoving(undefined);
      }
    }
  }

  function save() {
    if (pending || unknown.length) return;
    try {
      void observe(operations.command(captureMethod, { period }));
    } catch (error) {
      setMessage(`The capture could not be admitted: ${failure(error)}`);
    }
  }

  function requestRemoval() {
    if (!opened || pending || unknown.length) return;
    setMessage(undefined);
    setRemoving(structuredClone(opened.header));
  }

  function confirmRemoval() {
    if (!removing || pending || unknown.length) return;
    try {
      void observe(
        operations.command(deleteMethod, { id: removing.id }),
        removing,
      );
    } catch (error) {
      setRemoving(undefined);
      setMessage(`The removal could not be admitted: ${failure(error)}`);
    }
  }

  function dismissRemoval() {
    if (!pending) setRemoving(undefined);
  }

  const removalReturn = useCallback(() => {
    for (const element of [removeButton.current, backButton.current])
      if (element?.isConnected && !element.disabled) return element;
    return refreshButton.current;
  }, []);

  async function retire(command: (typeof unknown)[number]) {
    const signal = ownerSignal();
    setPending(true);
    try {
      await command.retire(signal);
      if (!signal.aborted)
        setMessage(
          "Original command retired. Any report already saved remains in the archive.",
        );
    } catch (error) {
      if (!signal.aborted)
        setMessage(
          `The original command could not be retired: ${failure(error)}`,
        );
    } finally {
      if (!signal.aborted) setPending(false);
    }
  }

  const headers = archive.accepted;
  return (
    <Panel
      ref={panel}
      label="Saved usage reports"
      header={
        <PanelHeader
          title="Saved usage reports"
          description={
            opened
              ? undefined
              : "Immutable copies of core accounting reports. Live accounting is unaffected."
          }
        />
      }
      controls={
        opened ? (
          <>
            <Toolbar
              label="Report navigation"
              actions={
                <Button
                  pending={opened.loading}
                  onClick={() => void read(opened.header.id)}
                >
                  Refresh report
                </Button>
              }
            >
              <Button
                ref={backButton}
                variant="ghost"
                size="sm"
                onClick={() => leave(activeElement(), true)}
              >
                <Icon glyph={ArrowLeft} size="sm" />
                Back to archive
              </Button>
            </Toolbar>
            <Toolbar label="Saved report">
              <PanelTitle ref={heading} tabIndex={-1} size="sm" aria-level={2}>
                {reportTitle(opened.header)}
              </PanelTitle>
            </Toolbar>
          </>
        ) : (
          <Toolbar label="Archive actions">
            <Select
              label="Capture period"
              value={period}
              onValueChange={(value) => setPeriod(value as Period)}
              options={[
                { value: "daily", label: "Daily · last 24 hours" },
                { value: "weekly", label: "Weekly · last 7 days" },
              ]}
            />
            <Button disabled={pending || unknown.length > 0} onClick={save}>
              Save report
            </Button>
            <Button
              ref={refreshButton}
              onClick={() => void refresh(ownerSignal())}
            >
              Refresh archive
            </Button>
          </Toolbar>
        )
      }
    >
      {message && <Metadata role="status">{message}</Metadata>}
      {unknown.map((command) => (
        <Toolbar key={command.idempotencyKey} label="Original archive command">
          <Metadata>Confirmation pending for the original command.</Metadata>
          <Button
            disabled={pending}
            onClick={() => void observe(command.recover(ownerSignal()))}
          >
            Check original outcome
          </Button>
          <Button disabled={pending} onClick={() => void retire(command)}>
            Retire original command
          </Button>
        </Toolbar>
      ))}
      {opened && (
        <SavedReport
          opened={opened}
          removeButton={removeButton}
          pending={pending || unknown.length > 0}
          remove={requestRemoval}
        />
      )}
      <div hidden={Boolean(opened)}>
        <PanelSection
          title="Archive"
          description={
            headers &&
            `${headers.length} saved reports. Capacity is 64; remove a report to make room.`
          }
        >
          <Metadata>
            A capture copies core accounting for the last 24 hours (daily) or
            the last 7 days (weekly) before it runs, not a calendar day or week.
          </Metadata>
          {archive.loading && !headers && (
            <Metadata role="status">Reading saved reports…</Metadata>
          )}
          {archive.error && (
            <Metadata role="alert">
              The archive could not be read: {archive.error}.
              {headers && " The last accepted archive remains shown."}
            </Metadata>
          )}
          {headers && !headers.length && (
            <EmptyState
              title="No saved reports"
              sub="Choose a period and save the current core report."
            />
          )}
          {headers && headers.length > 0 && (
            <RecordGroup variant="list" aria-label="Saved reports">
              {headers.map((header) => (
                <ListRow key={header.id}>
                  <RecordHeading
                    title={`${periods[header.period].label} · ${observed(header)}`}
                    description={`${periods[header.period].window} · Starts ${lowerBound(header)}`}
                    actions={
                      <Button
                        aria-label={`View report: ${reportTitle(header)}`}
                        onClick={(event) => {
                          setMessage(undefined);
                          open(header, event.currentTarget, activeElement());
                        }}
                      >
                        View report
                      </Button>
                    }
                  />
                </ListRow>
              ))}
            </RecordGroup>
          )}
        </PanelSection>
      </div>
      {removing && (
        <ModalDialog
          open
          role="alertdialog"
          title={`Remove ${reportTitle(removing)}?`}
          description="Only this saved copy is removed. Core accounting evidence and totals are unchanged, and the copy cannot be restored."
          closeLabel="Close removal confirmation"
          size="sm"
          initialFocus={cancelRemoval}
          finalFocus={removalReturn}
          dismissible={!pending}
          onOpenChange={(open) => {
            if (!open) dismissRemoval();
          }}
          footer={
            <>
              <Button
                ref={cancelRemoval}
                variant="ghost"
                disabled={pending}
                onClick={dismissRemoval}
              >
                Cancel
              </Button>
              <Button
                variant="tonal"
                tone="negative"
                pending={pending}
                aria-busy={pending}
                onClick={confirmRemoval}
              >
                {pending ? "Removing report…" : "Remove saved report"}
              </Button>
            </>
          }
        >
          <Facts
            values={[
              [
                "Period",
                `${periods[removing.period].label} · ${periods[removing.period].window}`,
              ],
              ["Core observation", observed(removing)],
              ["Actual lower bound", lowerBound(removing)],
            ]}
          />
        </ModalDialog>
      )}
    </Panel>
  );
}

function SavedReport({
  opened,
  removeButton,
  pending,
  remove,
}: {
  opened: OpenedReport;
  removeButton: Ref<HTMLButtonElement>;
  pending: boolean;
  remove: () => void;
}) {
  const { header, accepted: report } = opened;
  const summary = report?.summary;
  return (
    <>
      <Metadata>
        Saved copy of a core accounting report. It does not update when live
        accounting changes.
      </Metadata>
      <Facts
        values={[
          [
            "Period",
            `${periods[header.period].label} · ${periods[header.period].window}`,
          ],
          ["Core observation", observed(header)],
          ["Actual lower bound", lowerBound(header)],
        ]}
      />
      {opened.loading && (
        <Metadata role="status">Reading saved report…</Metadata>
      )}
      {opened.error && (
        <Metadata role="alert">
          {report
            ? `The saved report could not be refreshed: ${opened.error}. The last accepted report remains visible.`
            : `The saved report could not be read: ${opened.error}.`}
        </Metadata>
      )}
      {summary && (
        <PanelSection
          title="Saved core totals"
          description="Values are the saved core response. Unknown usage and prices retain their core meaning."
        >
          <Facts
            values={[
              ["Model attempts", count(summary.coverage.calls)],
              ["Known input tokens", count(summary.total.inputTokens)],
              ["Known output tokens", count(summary.total.outputTokens)],
              ["Known price subtotal (USD)", amount(summary.recordedCostUsd)],
              ["Total cost (USD)", amount(summary.total.costUsd)],
              [
                "Usage recorded",
                `${summary.coverage.reportedCalls}/${summary.coverage.calls}`,
              ],
              [
                "Priced attempts",
                `${summary.coverage.pricedCalls}/${summary.coverage.calls}`,
              ],
              ["Outcome unknown", count(summary.coverage.unknownCalls)],
            ]}
          />
          <Disclosure title="Daily core breakdown">
            <RecordGroup variant="list">
              {summary.byDay.map((day) => (
                <ListRow key={day.key}>
                  <RecordHeading
                    title={day.key}
                    description={`${count(day.coverage.calls)} attempts · ${amount(day.recordedCostUsd)} known USD`}
                  />
                </ListRow>
              ))}
            </RecordGroup>
          </Disclosure>
        </PanelSection>
      )}
      <Disclosure title="Exact report identity">
        <Facts
          values={[
            ["Report ID", header.id],
            ["Observed at (UTC)", header.observedAt],
            ["Lower bound (UTC)", header.since],
          ]}
        />
      </Disclosure>
      <Toolbar label="Saved report actions">
        <Button
          ref={removeButton}
          variant="tonal"
          tone="negative"
          disabled={pending}
          onClick={remove}
        >
          Remove saved report
        </Button>
      </Toolbar>
    </>
  );
}
