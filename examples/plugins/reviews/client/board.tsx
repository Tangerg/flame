import { useEffect, useMemo, useRef, useState } from "react";
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
  mutationSettlementIsUnknown,
  type ComponentContext,
} from "@flame/plugin-sdk";
import type { MutationPromise } from "@flame/runtime-contract/client";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";

const listMethod = "io.github.tangerg.flame.examples/reviews.list";
const updateMethod = "io.github.tangerg.flame.examples/reviews.update";
const analyzeMethod = "io.github.tangerg.flame.examples/reviews.analyzers";
type Review = MethodOutputs[typeof listMethod]["reviews"][number];
type UpdateResult = MethodOutputs[typeof updateMethod];
type Status = MethodInputs[typeof updateMethod]["status"];
type Analysis = MethodOutputs[typeof analyzeMethod];

export function Board({ context }: { context: ComponentContext }) {
  const identity = `${context.binding.installationId}/${context.binding.digest}/${context.binding.contribution}`;
  return <ReviewsBoard key={identity} context={context} />;
}

function ReviewsBoard({ context }: { context: ComponentContext }) {
  const operations = useMemo(
    () =>
      bindOperations<MethodInputs, MethodOutputs>(context, CAPABILITY_JSON, {
        scope: "installation",
        reference: context.binding.installationId,
      }),
    [context],
  );
  const [reviews, setReviews] = useState<Review[]>([]);
  const [analysis, setAnalysis] = useState<Analysis>();
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState<string>();
  const [unknown, setUnknown] = useState<MutationPromise<UpdateResult>>();
  const [status, setStatus] = useState<Status>("resolved");
  const lifetime = useRef<AbortController>(undefined);
  const reading = useRef<AbortController>(undefined);

  async function refresh(owner: AbortSignal) {
    reading.current?.abort();
    const request = new AbortController();
    reading.current = request;
    const signal = AbortSignal.any([owner, request.signal]);
    setLoading(true);
    try {
      const result = await operations.query(listMethod, {}, signal);
      if (signal.aborted) return;
      setReviews(result.reviews);
    } catch (error) {
      if (!signal.aborted)
        setMessage(
          `The current review could not be read: ${failureMessage(error)}`,
        );
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  }

  useEffect(() => {
    const owner = new AbortController();
    lifetime.current = owner;
    const signal = AbortSignal.any([context.signal, owner.signal]);
    void refresh(signal);
    return () => owner.abort();
  }, [operations, context.signal]);

  async function observe(
    command: MutationPromise<UpdateResult>,
    owner: AbortSignal,
  ) {
    setPending(true);
    setMessage(undefined);
    try {
      const result = await command;
      if (owner.aborted) return;
      setUnknown(undefined);
      setAnalysis(undefined);
      setMessage(
        result.type === "updated"
          ? "Status saved."
          : result.type === "revisionConflict"
            ? "The review changed after you read it. Inspect the current revision before submitting another change."
            : `The change was refused: ${result.type}.`,
      );
      await refresh(owner);
    } catch (error) {
      if (owner.aborted) return;
      if (mutationSettlementIsUnknown(error)) {
        setUnknown(() => command);
        setMessage(
          "Confirmation is unknown. Preserve this command and check its original outcome before submitting another change.",
        );
      } else setMessage(`The change failed: ${failureMessage(error)}`);
    } finally {
      if (!owner.aborted) setPending(false);
    }
  }

  function change(review: Review) {
    const owner = lifetime.current;
    if (!owner || pending || unknown) return;
    const input = { id: review.id, expectedRevision: review.revision, status };
    try {
      const command = operations.command(updateMethod, input);
      void observe(command, AbortSignal.any([owner.signal, context.signal]));
    } catch (error) {
      if (!context.signal.aborted && !owner.signal.aborted)
        setMessage(
          `The change could not be admitted: ${failureMessage(error)}`,
        );
    }
  }

  async function analyze() {
    const owner = lifetime.current;
    if (!owner || pending || loading) return;
    const signal = AbortSignal.any([owner.signal, context.signal]);
    setPending(true);
    try {
      const result = await operations.query(analyzeMethod, {}, signal);
      if (!signal.aborted) setAnalysis(result);
    } catch (error) {
      if (!signal.aborted)
        setMessage(`Analysis failed: ${failureMessage(error)}`);
    } finally {
      if (!signal.aborted) setPending(false);
    }
  }

  return (
    <Panel label="Reviews">
      <Toolbar label="Review actions">
        <PanelTitle>Reviews</PanelTitle>
        <Button
          disabled={loading || pending}
          onClick={() => {
            const owner = lifetime.current;
            if (owner)
              void refresh(AbortSignal.any([owner.signal, context.signal]));
          }}
        >
          Refresh
        </Button>
        <Button
          disabled={loading || pending || !!unknown}
          onClick={() => void analyze()}
        >
          Analyze
        </Button>
      </Toolbar>
      {loading && <p role="status">Reading current reviews…</p>}
      {message && <p role="status">{message}</p>}
      {unknown && (
        <Toolbar label="Original command recovery">
          <Metadata>Command: {unknown.idempotencyKey}</Metadata>
          <Button
            disabled={pending}
            onClick={() => {
              const owner = lifetime.current;
              if (owner)
                void observe(
                  unknown.retry({
                    signal: AbortSignal.any([owner.signal, context.signal]),
                  }),
                  AbortSignal.any([owner.signal, context.signal]),
                );
            }}
          >
            Check original outcome
          </Button>
        </Toolbar>
      )}
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
              {review.status} · Revision {review.revision} · {review.id}
            </Metadata>
            <Toolbar label={`Change ${review.title}`}>
              <Select
                label="New status"
                value={status}
                onValueChange={(value) => setStatus(value as Status)}
                options={[
                  { value: "open", label: "Open" },
                  { value: "resolved", label: "Resolved" },
                ]}
                disabled={pending || !!unknown}
              />
              <Button
                disabled={
                  pending || loading || !!unknown || review.status === status
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
          <h2>Findings for revision {analysis.review.revision}</h2>
          {!analysis.findings.length && <p>No applicable findings.</p>}
          <ul>
            {analysis.findings.map((finding, index) => (
              <li key={`${finding.code}/${index}`}>
                <RecordBody>{finding.message}</RecordBody>
                <Metadata>
                  {finding.code} · {finding.reviewId} · Revision{" "}
                  {finding.revision}
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
