import { useEffect, useState } from "react";
import { failureText, type Translate } from "@flame/component-sdk";
import {
  Badge,
  Button,
  Callout,
  Disclosure,
  EvidenceImage,
  Facts,
  Form,
  FormActions,
  FormField,
  Metadata,
  RecordBody,
  RecordGroup,
  Select,
  TextArea,
  Toolbar,
  imageSizeFromBytes,
  type PixelSize,
} from "@flame/ui";
import { FIELD_LIMITS } from "../contracts/limits";
import { METHODS } from "../contracts/methods";
import { type Check, type RenderReport } from "../contracts/evidence";
import type { Asset, Assessment, DocumentRead } from "./types";
import type { DeliverablesState } from "./useDeliverables";
import { downloadAsset } from "./transfer";
import { Source } from "./provenance";

// The part of the deliverables state this view reads.
export type EvidenceState = Pick<
  DeliverablesState,
  "blocked" | "run" | "selectRevision" | "operations" | "signal"
>;

export function Findings({ checks, t }: { checks: Check[]; t: Translate }) {
  return (
    <RecordGroup aria-label={t("checks.title")}>
      {!checks.length && <Metadata>{t("checks.none")}</Metadata>}
      {checks.map((check, index) => (
        <Callout
          key={`${check.code}:${index}`}
          kind={
            check.severity === "error"
              ? "caution"
              : check.severity === "warning"
                ? "warning"
                : "note"
          }
        >
          <Badge
            tone={
              check.severity === "error"
                ? "negative"
                : check.severity === "warning"
                  ? "warning"
                  : "neutral"
            }
          >
            {t(`severity.${check.severity}`)}
          </Badge>
          <RecordBody>{check.message}</RecordBody>
          <Metadata>
            {check.code}
            {check.location ? ` · ${check.location}` : ""}
          </Metadata>
        </Callout>
      ))}
    </RecordGroup>
  );
}

function PageImage({
  state,
  asset,
  page,
  t,
  loaded,
}: {
  state: EvidenceState;
  asset: Asset;
  page: number;
  t: Translate;
  loaded(): void;
}) {
  const [read, setRead] = useState<{
    id: string;
    url?: string;
    size?: PixelSize;
    error?: string;
  }>();
  const { operations, signal } = state;
  useEffect(() => {
    const request = new AbortController();
    const lifetime = AbortSignal.any([signal, request.signal]);
    let url: string | undefined;
    setRead({ id: asset.id });
    void downloadAsset(
      (input, owned) => operations.query(METHODS.download, input, owned),
      asset,
      lifetime,
    )
      .then((bytes) => {
        if (lifetime.aborted) return;
        url = URL.createObjectURL(new Blob([bytes], { type: "image/png" }));
        setRead({
          id: asset.id,
          url,
          size: imageSizeFromBytes(bytes) ?? undefined,
        });
      })
      .catch((error: unknown) => {
        if (!lifetime.aborted)
          setRead({ id: asset.id, error: failureText(error) });
      });
    return () => {
      request.abort();
      if (url) URL.revokeObjectURL(url);
    };
  }, [
    operations,
    signal,
    asset.id,
    asset.sha256,
    asset.size,
    asset.filename,
    asset.mime,
  ]);
  const current = read?.id === asset.id ? read : undefined;
  return (
    <RecordGroup aria-label={t("preview.page", { page })}>
      {!current?.url && !current?.error && (
        <Metadata role="status">{t("preview.loading")}</Metadata>
      )}
      {current?.error && <Callout kind="warning">{current.error}</Callout>}
      {current?.url && (
        <RecordGroup
          onLoadCapture={loaded}
          onErrorCapture={() =>
            setRead({ id: asset.id, error: t("preview.imageFailed") })
          }
        >
          <EvidenceImage
            src={current.url}
            width={current.size?.width}
            height={current.size?.height}
            decoding="async"
            alt={t("preview.page", { page })}
            labels={{
              view: t("image.view"),
              close: t("image.close"),
              fit: t("image.fit"),
              zoomIn: t("image.zoomIn"),
              zoomOut: t("image.zoomOut"),
              description: t("image.description"),
            }}
          />
        </RecordGroup>
      )}
    </RecordGroup>
  );
}

export function RenderedPages({
  state,
  document,
  assessment,
  report,
  t,
}: {
  state: EvidenceState;
  document: DocumentRead;
  assessment: Assessment;
  report: RenderReport;
  t: Translate;
}) {
  const [page, setPage] = useState(report.pages[0]?.number ?? 1);
  const [loadedPages, setLoadedPages] = useState<number[]>([]);
  const [reviewedPages, setReviewedPages] = useState<number[]>([]);
  const [verdict, setVerdict] = useState<"accepted" | "changes_requested">(
    "changes_requested",
  );
  const [notes, setNotes] = useState("");
  const [sent, setSent] = useState(false);
  const selected = report.pages.find((entry) => entry.number === page);
  const asset = assessment.assets.find(
    (entry) => entry.id === selected?.assetId,
  );
  const position = report.pages.findIndex((entry) => entry.number === page);
  const previousPage = report.pages[position - 1];
  const nextPage = report.pages[position + 1];
  const valid =
    selected &&
    asset &&
    asset.mime === "image/png" &&
    asset.sha256 === selected.sha256;
  return (
    <RecordGroup>
      <Metadata>{t("preview.help")}</Metadata>
      {report.pages.length ? (
        <Toolbar label={t("preview.navigation")}>
          <Button
            disabled={!previousPage}
            onClick={() => {
              if (previousPage) setPage(previousPage.number);
            }}
          >
            {t("previous")}
          </Button>
          <Select
            label={t("preview.pageLabel")}
            value={String(page)}
            onValueChange={(value) => setPage(Number(value))}
            options={report.pages.map((entry) => ({
              value: String(entry.number),
              label: t("preview.page", { page: entry.number }),
            }))}
          />
          <Button
            disabled={position < 0 || !nextPage}
            onClick={() => {
              if (nextPage) setPage(nextPage.number);
            }}
          >
            {t("next")}
          </Button>
        </Toolbar>
      ) : (
        <Callout kind="warning">{t("preview.empty")}</Callout>
      )}
      {valid ? (
        <PageImage
          key={asset.id}
          state={state}
          asset={asset}
          page={page}
          t={t}
          loaded={() =>
            setLoadedPages((pages) =>
              pages.includes(page) ? pages : [...pages, page],
            )
          }
        />
      ) : (
        selected && <Callout kind="warning">{t("preview.invalid")}</Callout>
      )}
      <Findings checks={report.checks} t={t} />
      <Metadata>{t("preview.fonts")}</Metadata>
      <Disclosure title={t("review.title")} keepMounted>
        <Metadata>{t("review.help")}</Metadata>
        <Toolbar label={t("review.pages")}>
          <Button
            disabled={
              state.blocked ||
              !loadedPages.includes(page) ||
              reviewedPages.includes(page)
            }
            onClick={() => {
              setReviewedPages((pages) =>
                [...pages, page].sort((a, b) => a - b),
              );
              setSent(false);
            }}
          >
            {t("review.addPage", { page })}
          </Button>
          {reviewedPages.map((number) => (
            <Button
              key={number}
              disabled={state.blocked}
              onClick={() => {
                setReviewedPages((pages) =>
                  pages.filter((value) => value !== number),
                );
                setSent(false);
              }}
              aria-label={t("review.removePage", { page: number })}
            >
              {t("preview.page", { page: number })} ×
            </Button>
          ))}
        </Toolbar>
        <Form
          onSubmit={(event) => {
            event.preventDefault();
            if (!reviewedPages.length || !notes.trim() || state.blocked) return;
            void state.run(
              METHODS.review,
              {
                id: document.document.id,
                revision: document.revision.id,
                renderId: assessment.id,
                pages: [...reviewedPages],
                verdict,
                notes: notes.trim(),
              },
              () => setSent(true),
            );
          }}
        >
          <Select
            label={t("review.verdict")}
            value={verdict}
            disabled={state.blocked}
            options={[
              {
                value: "changes_requested",
                label: t("review.changes_requested"),
              },
              { value: "accepted", label: t("review.accepted") },
            ]}
            onValueChange={(value) => {
              setVerdict(value as typeof verdict);
              setSent(false);
            }}
          />
          <FormField label={t("review.notes")}>
            <TextArea
              value={notes}
              required
              maxLength={FIELD_LIMITS.reviewNotes}
              rows={5}
              disabled={state.blocked}
              onChange={(event) => {
                setNotes(event.target.value);
                setSent(false);
              }}
            />
          </FormField>
          <FormActions
            label={t("review.actions")}
            feedback={
              sent && <Metadata role="status">{t("review.recorded")}</Metadata>
            }
          >
            <Button
              type="submit"
              variant="primary"
              disabled={
                state.blocked || !reviewedPages.length || !notes.trim() || sent
              }
            >
              {t("review.submit")}
            </Button>
          </FormActions>
        </Form>
      </Disclosure>
      <Disclosure title={t("preview.details")}>
        <Facts
          values={[
            [t("revision.id"), report.sourceRevision],
            [t("file.hash"), report.sourceSha256],
            [t("preview.platform"), report.platform],
            [
              t("preview.engines"),
              report.engines
                .map((entry) => `${entry.name} ${entry.version}`)
                .join(" · "),
            ],
          ]}
        />
        <Source label={t("preview.extracted")} value={report.extractedText} />
      </Disclosure>
    </RecordGroup>
  );
}
