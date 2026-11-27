import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import {
  Button,
  EmptyState,
  Metadata,
  Panel,
  PanelTitle,
  RecordBody,
  RecordGroup,
  Select,
  Toolbar,
} from "@flame/ui";
import {
  bindOperations,
  useContextState,
  type ComponentContext,
} from "@flame/component-sdk";
import {
  ANALYZERS_JSON,
  analyzeMethod,
  listMethod,
  updateMethod,
  reviewStatuses,
} from "./board.generated";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";

type Review = MethodOutputs[typeof listMethod]["reviews"][number];
type UpdateResult = MethodOutputs[typeof updateMethod];
type Status = MethodInputs[typeof updateMethod]["status"];
const statusLabels = { open: "Open", resolved: "Resolved" } satisfies Record<
  Status,
  string
>;
const statusOptions = reviewStatuses.map((value) => ({
  value,
  label: statusLabels[value],
}));
type Analysis = MethodOutputs[typeof analyzeMethod];
type AnalyzerRead =
  { type: "ready"; available: boolean } | { type: "failed"; message: string };

type BoundContext = ComponentContext & {
  readonly binding: NonNullable<ComponentContext["binding"]>;
};
type Inspection = {
  pending: boolean;
  reviews: Review[];
  analyzers: AnalyzerRead;
};

export function Board({ context }: { context: ComponentContext }) {
  const bound = useMemo(() => {
    if (!context.binding)
      throw new Error("Reviews requires its declared callable contribution");
    return { ...context, binding: context.binding };
  }, [context]);
  const identity = `${bound.binding.installationId}/${bound.binding.digest}/${bound.binding.contribution}`;
  return <ReviewsBoard key={identity} context={bound} />;
}

function ReviewsBoard({ context }: { context: BoundContext }) {
  const operations = useMemo(
    () =>
      bindOperations<MethodInputs, MethodOutputs>(context, CAPABILITY_JSON, {
        scope: "installation",
        reference: context.binding.installationId,
      }),
    [context],
  );
  const [current, setInspection] = useContextState<Inspection>(context);
  const reviews = current?.reviews ?? [];
  const analyzers = current?.analyzers;
  const loading = current === undefined || current.pending;
  const [analysis, setAnalysis] = useState<Analysis>();
  const [analyzing, setAnalyzing] = useState(false);
  const [message, setMessage] = useState<string>();
  const [recoveryError, setRecoveryError] = useState<{ key: string; text: string }>();
  useSyncExternalStore(operations.subscribeCommands, operations.commandsVersion);
  const readCommands = () => context.signal.aborted ? [] : operations.commands(updateMethod);
  const commands = readCommands();
  const locked = analyzing || commands.length > 0;
  const recoverable = commands.filter((command) => command.state === "recoverable");
  const [status, setStatus] = useState<Status>("resolved");
  const lifetime = useRef<AbortController>(undefined);
  const reading = useRef<AbortController>(undefined);

  const refresh = useCallback(
    (owner: AbortSignal) => {
      reading.current?.abort();
      const request = new AbortController();
      reading.current = request;
      const signal = AbortSignal.any([owner, request.signal]);
      return operations
        .query(listMethod, {}, signal)
        .then(async (result) => {
          let analyzers: AnalyzerRead = { type: "ready", available: false };
          const review = result.reviews[0];
          if (review) {
            try {
              const values = await context.track(() =>
                context.runtime.plugins.resolvePoint(
                  {
                    contract: ANALYZERS_JSON,
                    target: {
                      scope: "installation",
                      reference: context.binding.installationId,
                    },
                    attributes: JSON.stringify({ reviewStatus: review.status }),
                    order: [],
                  },
                  signal,
                ),
              );
              analyzers = { type: "ready", available: values.data.length > 0 };
            } catch (error) {
              analyzers = { type: "failed", message: failureMessage(error) };
            }
          }
          if (signal.aborted) return;
          setInspection({
            pending: false,
            reviews: result.reviews,
            analyzers,
          });
        })
        .catch((error) => {
          if (!signal.aborted) {
            setMessage(
              `The current review could not be read: ${failureMessage(error)}`,
            );
            setInspection((previous) => ({
              pending: false,
              reviews: previous?.reviews ?? [],
              analyzers: previous?.analyzers ?? {
                type: "ready",
                available: false,
              },
            }));
          }
        });
    },
    [context, operations, setInspection],
  );

  function reload(owner: AbortSignal) {
    setInspection((previous) => ({
      pending: true,
      reviews: previous?.reviews ?? [],
      analyzers: previous?.analyzers ?? { type: "ready", available: false },
    }));
    return refresh(owner);
  }

  useEffect(() => {
    const owner = new AbortController();
    lifetime.current = owner;
    const signal = AbortSignal.any([context.signal, owner.signal]);
    void refresh(signal);
    return () => owner.abort();
  }, [refresh, context.signal]);

  async function observe(
    key: string,
    attempt: () => Promise<UpdateResult>,
    owner: AbortSignal,
  ) {
    setMessage(undefined);
    setRecoveryError(undefined);
    try {
      const result = await attempt();
      if (owner.aborted) return;
      setAnalysis(undefined);
      setMessage(
        result.type === "updated"
          ? "Status saved."
          : result.type === "revisionConflict"
            ? "The review changed after you read it. Inspect the current revision before submitting another change."
            : `The change was refused: ${result.type}.`,
      );
      await reload(owner);
    } catch (error) {
      if (owner.aborted) return;
      const text = failureMessage(error);
      if (readCommands().some((command) => command.idempotencyKey === key && command.state === "recoverable"))
        setRecoveryError({ key, text });
      else setMessage(`The change failed: ${text}`);
    }
  }

  function change(review: Review) {
    const owner = lifetime.current;
    if (!owner || owner.signal.aborted || analyzing) return;
    const input = { id: review.id, expectedRevision: review.revision, status };
    try {
      if (readCommands().length > 0) return;
      const command = operations.command(updateMethod, input);
      void observe(
        command.idempotencyKey,
        () => command,
        AbortSignal.any([owner.signal, context.signal]),
      );
    } catch (error) {
      if (!context.signal.aborted && !owner.signal.aborted)
        setMessage(
          `The change could not be admitted: ${failureMessage(error)}`,
        );
    }
  }

  function recover(key: string) {
    const owner = lifetime.current;
    if (!owner || owner.signal.aborted || analyzing) return;
    const original = readCommands().find((command) => command.idempotencyKey === key);
    if (original?.state !== "recoverable") return;
    const signal = AbortSignal.any([owner.signal, context.signal]);
    void observe(key, () => original.recover(signal), signal);
  }

  async function analyze() {
    const owner = lifetime.current;
    if (
      !owner ||
      analyzing ||
      readCommands().length > 0 ||
      loading ||
      analyzers?.type !== "ready" ||
      !analyzers.available
    )
      return;
    const signal = AbortSignal.any([owner.signal, context.signal]);
    setAnalyzing(true);
    try {
      const result = await operations.query(analyzeMethod, {}, signal);
      if (!signal.aborted) setAnalysis(result);
    } catch (error) {
      if (!signal.aborted)
        setMessage(`Analysis failed: ${failureMessage(error)}`);
    } finally {
      if (!signal.aborted) setAnalyzing(false);
    }
  }

  return (
    <Panel label="Reviews">
      <Toolbar label="Review actions">
        <PanelTitle>Reviews</PanelTitle>
        <Button
          disabled={loading || analyzing}
          onClick={() => {
            const owner = lifetime.current;
            if (owner)
              void reload(AbortSignal.any([owner.signal, context.signal]));
          }}
        >
          Refresh
        </Button>
        {analyzers?.type === "ready" && analyzers.available && (
          <Button
            disabled={loading || locked}
            onClick={() => void analyze()}
          >
            Analyze
          </Button>
        )}
      </Toolbar>
      {loading && <p role="status">Reading current reviews…</p>}
      {message && <p role="status">{message}</p>}
      {analyzers?.type === "failed" && (
        <p role="alert">
          Analyzer actions could not be resolved: {analyzers.message}
        </p>
      )}
      {recoverable.map((command) => (
        <Toolbar key={command.idempotencyKey} label="Original command recovery">
          <Metadata>Command: {command.idempotencyKey}</Metadata>
          {recoveryError?.key === command.idempotencyKey && (
            <p role="alert">{recoveryError.text}</p>
          )}
          <Button
            disabled={analyzing}
            onClick={() => recover(command.idempotencyKey)}
          >
            Check original outcome
          </Button>
        </Toolbar>
      ))}
      <RecordGroup aria-label="Current reviews">
        {!loading && !reviews.length && (
          <EmptyState
            title="No reviews"
            sub="Refresh to read the current owner."
          />
        )}
        {reviews.map((review) => (
          <section key={review.id} aria-label={review.title}>
            <RecordBody>{review.title}</RecordBody>
            <Metadata>
              {review.status} · Revision {String(review.revision)} · {review.id}
            </Metadata>
            <Toolbar label={`Change ${review.title}`}>
              <Select
                label="New status"
                value={status}
                onValueChange={(value) => {
                  const selected = reviewStatuses.find(
                    (status) => status === value,
                  );
                  if (selected !== undefined) setStatus(selected);
                }}
                options={statusOptions}
                disabled={locked}
              />
              <Button
                disabled={
                  locked ||
                  loading ||
                  review.status === status
                }
                onClick={() => change(review)}
              >
                Save status
              </Button>
            </Toolbar>
          </section>
        ))}
      </RecordGroup>
      {analysis && (
        <section aria-label="Analyzer findings">
          <h2>Findings for revision {String(analysis.review.revision)}</h2>
          {!analysis.findings.length && <p>No applicable findings.</p>}
          <ul>
            {analysis.findings.map((finding, index) => (
              <li key={`${finding.code}/${index}`}>
                <RecordBody>{finding.message}</RecordBody>
                <Metadata>
                  {finding.code} · {finding.reviewId} · Revision{" "}
                  {String(finding.revision)}
                </Metadata>
              </li>
            ))}
          </ul>
        </section>
      )}
    </Panel>
  );
}

function failureMessage(error: unknown) {
  return error instanceof Error
    ? error.message
    : "The operation could not be completed.";
}
