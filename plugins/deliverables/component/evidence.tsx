import { useEffect, useMemo, useState } from "react";
import { failureText, type Translate } from "@flame/component-sdk";
import {
  Button,
  Callout,
  Disclosure,
  Facts,
  Metadata,
  RecordBody,
  RecordGroup,
  Select,
  Toolbar,
} from "@flame/ui";
import { METHODS } from "../contracts/methods";
import { renderReportSchema, reviewReportSchema } from "../contracts/evidence";
import type { Assessment, DocumentRead } from "./types";
import { type EvidenceState, RenderedPages } from "./renderedPages";

function useAssessment(
  state: EvidenceState,
  document: DocumentRead,
  id?: string,
) {
  const [read, setRead] = useState<{
    id: string;
    value?: Assessment;
    error?: string;
    loading: boolean;
  }>();
  const { operations, signal } = state;
  const documentId = document.document.id;
  const revision = document.revision.id;
  const hash = document.revision.sha256;
  useEffect(() => {
    if (!id) return;
    const request = new AbortController();
    const lifetime = AbortSignal.any([signal, request.signal]);
    setRead({ id, loading: true });
    void operations
      .query(
        METHODS.assessment,
        { id: documentId, revision, assessmentId: id },
        lifetime,
      )
      .then((value) => {
        if (lifetime.aborted) return;
        if (
          value.id !== id ||
          value.revision !== revision ||
          value.sourceSha256 !== hash
        )
          throw new Error(
            "deliverables: assessment differs from the captured revision",
          );
        setRead({ id, value, loading: false });
      })
      .catch((error: unknown) => {
        if (!lifetime.aborted)
          setRead({ id, error: failureText(error), loading: false });
      });
    return () => request.abort();
  }, [operations, signal, documentId, revision, hash, id]);
  return read?.id === id ? read : undefined;
}

function ReviewRecord({
  state,
  document,
  id,
  t,
}: {
  state: EvidenceState;
  document: DocumentRead;
  id: string;
  t: Translate;
}) {
  const read = useAssessment(state, document, id);
  const parsed = useMemo(() => {
    if (!read?.value) return;
    try {
      if (read.value.kind !== "review")
        throw new Error("deliverables: assessment is not a review");
      const report = reviewReportSchema.parse(
        JSON.parse(read.value.reportJSON),
      );
      if (
        report.sourceRevision !== document.revision.id ||
        report.sourceSha256 !== document.revision.sha256
      )
        throw new Error(
          "deliverables: review report differs from the captured revision",
        );
      return { report };
    } catch (error) {
      return { error: failureText(error) };
    }
  }, [read?.value, document.revision.id, document.revision.sha256]);
  return (
    <RecordGroup>
      {read?.loading && <Metadata role="status">{t("reading")}</Metadata>}
      {(read?.error || parsed?.error) && (
        <Callout kind="warning">{read?.error ?? parsed?.error}</Callout>
      )}
      {parsed?.report && (
        <>
          <Facts
            values={[
              [t("review.verdict"), t(`review.${parsed.report.verdict}`)],
              [t("review.pages"), parsed.report.pages.join(", ")],
              [
                t("review.origin"),
                t(`origin.${parsed.report.reviewer.origin}`),
              ],
              [t("preview.renderId"), parsed.report.renderId],
            ]}
          />
          <RecordBody>{parsed.report.notes}</RecordBody>
        </>
      )}
    </RecordGroup>
  );
}

export function EvidencePanel({
  state,
  document,
  t,
}: {
  state: EvidenceState;
  document: DocumentRead;
  t: Translate;
}) {
  const [renderId, setRenderId] = useState<string>();
  const [reviewId, setReviewId] = useState<string>();
  const renders = document.assessments.filter(
    (entry) => entry.kind === "render",
  );
  const reviews = document.assessments.filter(
    (entry) => entry.kind === "review",
  );
  useEffect(() => {
    if (!renderId && renders[0]) setRenderId(renders[0].id);
  }, [renderId, renders]);
  const read = useAssessment(state, document, renderId);
  const parsed = useMemo(() => {
    if (!read?.value) return;
    try {
      if (read.value.kind !== "render")
        throw new Error("deliverables: assessment is not a render");
      const report = renderReportSchema.parse(
        JSON.parse(read.value.reportJSON),
      );
      if (
        report.sourceRevision !== document.revision.id ||
        report.sourceSha256 !== document.revision.sha256 ||
        new Set(report.pages.map((entry) => entry.number)).size !==
          report.pages.length
      )
        throw new Error(
          "deliverables: render report differs from the captured revision",
        );
      return { report };
    } catch (error) {
      return { error: failureText(error) };
    }
  }, [read?.value, document.revision.id, document.revision.sha256]);
  return (
    <RecordGroup>
      <Toolbar label={t("preview.actions")}>
        <Button
          disabled={state.blocked}
          onClick={() => {
            void state.run(
              METHODS.render,
              { id: document.document.id, revision: document.revision.id },
              (value) => setRenderId(value.id),
            );
          }}
        >
          {t("preview.render")}
        </Button>
        {document.revision.format === "xlsx" && (
          <Button
            disabled={
              state.blocked || document.document.head !== document.revision.id
            }
            onClick={() => {
              const captured = {
                id: document.document.id,
                revision: document.revision.id,
              };
              void state.run(
                METHODS.recalculate,
                { id: captured.id, expectedRevision: captured.revision },
                (value) => state.selectRevision(value, captured),
              );
            }}
          >
            {t("preview.recalculate")}
          </Button>
        )}
      </Toolbar>
      {!renders.length && <Metadata>{t("preview.notRendered")}</Metadata>}
      {renders.length > 0 && (
        <Select
          label={t("preview.renderChoice")}
          value={renderId ?? null}
          onValueChange={setRenderId}
          options={renders.map((entry) => ({
            value: entry.id,
            label: `${entry.createdAt} · ${entry.id.slice(0, 12)}`,
          }))}
        />
      )}
      {read?.loading && <Metadata role="status">{t("reading")}</Metadata>}
      {(read?.error || parsed?.error) && (
        <Callout kind="warning">{read?.error ?? parsed?.error}</Callout>
      )}
      {read?.value && parsed?.report && (
        <RenderedPages
          key={read.value.id}
          state={state}
          document={document}
          assessment={read.value}
          report={parsed.report}
          t={t}
        />
      )}
      <Disclosure title={t("review.history")}>
        {!reviews.length && <Metadata>{t("review.none")}</Metadata>}
        {reviews.length > 0 && (
          <Select
            label={t("review.choose")}
            value={reviewId ?? null}
            onValueChange={setReviewId}
            options={reviews.map((entry) => ({
              value: entry.id,
              label: `${entry.createdAt} · ${entry.id.slice(0, 12)}`,
            }))}
          />
        )}
        {reviewId && (
          <ReviewRecord
            key={reviewId}
            state={state}
            document={document}
            id={reviewId}
            t={t}
          />
        )}
      </Disclosure>
    </RecordGroup>
  );
}
