import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  bindOperations,
  mutationSettlementIsUnknown,
  type ComponentContext,
} from "@flame/plugin-sdk";
import type { JSONNumber } from "@flame/runtime-contract/client";
import {
  Button,
  Disclosure,
  EmptyState,
  Facts,
  ListRow,
  Metadata,
  Panel,
  PanelHeader,
  PanelSection,
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
  const [headers, setHeaders] = useState<Header[]>([]);
  const [report, setReport] = useState<Report>();
  const [period, setPeriod] = useState<Period>("daily");
  const [reading, setReading] = useState(false);
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState<string>();
  const lifetime = useRef<AbortController>(undefined);
  const inspecting = useRef<AbortController>(undefined);
  const listing = useRef<AbortController>(undefined);
  const unknown = [
    ...operations.pendingCommands(captureMethod),
    ...operations.pendingCommands(deleteMethod),
  ];

  const refresh = useCallback(
    async (owner: AbortSignal) => {
      listing.current?.abort();
      const request = new AbortController();
      listing.current = request;
      const signal = AbortSignal.any([owner, request.signal]);
      try {
        const result = await operations.query(listMethod, {}, signal);
        if (!signal.aborted) setHeaders(result);
      } catch (error) {
        if (!signal.aborted)
          setMessage(`The archive could not be read: ${failure(error)}`);
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

  function ownerSignal() {
    const owner = lifetime.current;
    if (!owner) throw new Error("Report component lifetime has not started");
    return AbortSignal.any([context.signal, owner.signal]);
  }

  async function inspect(id: string) {
    inspecting.current?.abort();
    const request = new AbortController();
    inspecting.current = request;
    const signal = AbortSignal.any([ownerSignal(), request.signal]);
    setReading(true);
    try {
      const result = await operations.query(getMethod, { id }, signal);
      if (!signal.aborted) setReport(result);
    } catch (error) {
      if (!signal.aborted)
        setMessage(
          `The selected report could not be read: ${failure(error)}. The last accepted report remains visible.`,
        );
    } finally {
      if (!signal.aborted) setReading(false);
    }
  }

  async function observe(command: Promise<Header | boolean>) {
    const signal = ownerSignal();
    setPending(true);
    setMessage(undefined);
    try {
      const result = await command;
      if (signal.aborted) return;
      if (typeof result === "boolean") {
        setReport(undefined);
        setMessage("Saved report removed. Core accounting is unchanged.");
        await refresh(signal);
      } else {
        setMessage("Core report saved.");
        await Promise.all([refresh(signal), inspect(result.id)]);
      }
    } catch (error) {
      if (!signal.aborted)
        setMessage(
          mutationSettlementIsUnknown(error)
            ? "Confirmation is unknown. Check the original outcome before submitting another archive change."
            : `The archive change failed: ${failure(error)}`,
        );
    } finally {
      if (!signal.aborted) setPending(false);
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

  function remove() {
    if (!report || pending || unknown.length) return;
    try {
      void observe(operations.command(deleteMethod, { id: report.id }));
    } catch (error) {
      setMessage(`The removal could not be admitted: ${failure(error)}`);
    }
  }

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

  return (
    <Panel
      label="Saved usage reports"
      header={
        <PanelHeader
          title="Saved usage reports"
          description="Immutable copies of core accounting reports."
        />
      }
    >
      <Toolbar label="Archive actions">
        <Select
          label="Capture period"
          value={period}
          onValueChange={(value) => setPeriod(value as Period)}
          options={[
            { value: "daily", label: "Daily" },
            { value: "weekly", label: "Weekly" },
          ]}
        />
        <Button disabled={pending || unknown.length > 0} onClick={save}>
          Save report
        </Button>
        <Button disabled={pending} onClick={() => void refresh(ownerSignal())}>
          Refresh archive
        </Button>
      </Toolbar>
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
      <PanelSection
        title="Archive"
        description={`${headers.length} saved reports. Capacity is 64; remove a report to make room.`}
      >
        {!headers.length && (
          <EmptyState
            title="No saved reports"
            sub="Choose a period and save the current core report."
          />
        )}
        <RecordGroup variant="list" aria-label="Saved reports">
          {headers.map((header) => (
            <ListRow key={header.id}>
              <RecordHeading
                title={`${header.period === "daily" ? "Daily" : "Weekly"} · ${observed(header)}`}
                description={
                  header.since
                    ? `Core range starts ${new Date(header.since).toLocaleString()}`
                    : "All retained accounting"
                }
                actions={
                  <Button
                    disabled={pending || reading}
                    onClick={() => void inspect(header.id)}
                  >
                    View report
                  </Button>
                }
              />
            </ListRow>
          ))}
        </RecordGroup>
      </PanelSection>
      {reading && <Metadata role="status">Reading saved report…</Metadata>}
      {report && (
        <SavedReport
          report={report}
          pending={pending || unknown.length > 0}
          remove={remove}
        />
      )}
    </Panel>
  );
}

function SavedReport({
  report,
  pending,
  remove,
}: {
  report: Report;
  pending: boolean;
  remove: () => void;
}) {
  const summary = report.summary;
  return (
    <PanelSection
      title={`${report.period === "daily" ? "Daily" : "Weekly"} report`}
      description={`Core observation: ${observed(report)}`}
    >
      <Metadata>
        Range starts{" "}
        {report.since
          ? new Date(report.since).toLocaleString()
          : "at the beginning of retained evidence"}
        . Values below are the saved core response.
      </Metadata>
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
      <Metadata>
        Unknown usage and prices retain their core meaning. Saved reports do not
        update when current accounting changes.
      </Metadata>
      <Button disabled={pending} onClick={remove}>
        Remove saved report
      </Button>
    </PanelSection>
  );
}
