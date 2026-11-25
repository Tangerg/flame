import { useState, type KeyboardEvent, type ReactNode } from "react";
import { type ComposerReference, type Translate } from "@flame/component-sdk";
import {
  Button,
  Callout,
  Facts,
  FormActions,
  Metadata,
  RecordGroup,
  Toolbar,
} from "@flame/ui";
import { type ChartData, type Frame } from "../contracts/content";
import { type ElementView, type SlideView } from "../contracts/layouts";
import type { DocumentRead } from "./types";
import { label } from "./slides-presentation";
import { selectionPreamble } from "./live";
import { ChartFields, FrameFields, RunField } from "./slides-fields";

export function selectionReference(
  document: DocumentRead,
  slide: SlideView,
  element: ElementView | undefined,
  t: Translate,
): ComposerReference {
  const name = [
    document.document.title,
    t("slides.slide", { number: slide.number }),
    ...(element ? [label(element, t)] : []),
  ]
    .join(" · ")
    .replace(/\s+/gu, " ")
    .trim();
  const text = [
    ...selectionPreamble(document),
    `Slide ${slide.number}${slide.name ? ` ${JSON.stringify(slide.name)}` : ""}: ${slide.id}`,
    ...(element
      ? [
          `Element ${JSON.stringify(element.name)} (${element.kind}): ${element.id}`,
        ]
      : []),
    "Read this exact revision with the Deliverables slides tool and carry its current values as expect when editing.",
  ].join("\n");
  return { label: name, text };
}

export function editableTarget(event: KeyboardEvent): boolean {
  const target = event.target as HTMLElement;
  return (
    target.isContentEditable ||
    ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
  );
}

export function ElementInspector({
  t,
  element,
  original,
  editable,
  revertible,
  dirty,
  reference,
  onFrame,
  onText,
  onChart,
  onDelete,
  onRevert,
}: {
  t: Translate;
  element: ElementView;
  original: ElementView | undefined;
  editable: boolean;
  revertible: boolean;
  dirty: boolean;
  reference: ReactNode;
  onFrame(frame: Frame): void;
  onText(id: string, expect: string, text: string): void;
  onChart(expect: ChartData, data: ChartData): void;
  onDelete(): void;
  onRevert(): void;
}) {
  return (
    <RecordGroup>
      <Facts
        values={[
          [t("slides.element"), t(`slides.kind.${element.kind}`)],
          [t("slides.name"), element.name || null],
        ]}
      />
      {element.frameSource && element.frameSource !== "slide" && (
        <Metadata>{t(`slides.inherited.${element.frameSource}`)}</Metadata>
      )}
      {element.frame && (
        <FrameFields
          frame={element.frame}
          editable={editable}
          onCommit={onFrame}
          t={t}
        />
      )}
      {element.text.map((run, index) => {
        const saved =
          original?.text.find((entry) => entry.id === run.id)?.text ?? run.text;
        return (
          <RunField
            key={run.id}
            label={t("slides.run", { number: index + 1 })}
            text={run.text}
            editable={editable}
            onCommit={(text) => onText(run.id, saved, text)}
          />
        );
      })}
      {element.kind === "chart" && !element.chart && (
        <Metadata>{t("slides.chart.unsupported")}</Metadata>
      )}
      {element.chart && original?.chart && (
        <ChartFields
          chart={element.chart}
          editable={editable}
          onCommit={(data) => onChart(original.chart!.data, data)}
          t={t}
        />
      )}
      <Toolbar label={t("slides.elementActions")}>
        <Button disabled={!editable} onClick={onDelete}>
          {t("slides.delete")}
        </Button>
        <Button disabled={!revertible} onClick={onRevert}>
          {t("slides.revert")}
        </Button>
        {reference}
      </Toolbar>
      {dirty && <Metadata>{t("handoff.unsaved")}</Metadata>}
    </RecordGroup>
  );
}

export function SlideInspector({
  t,
  slide,
  saved,
  slideCount,
  editable,
  dirty,
  reference,
  onText,
  onMove,
  onRestructure,
}: {
  t: Translate;
  slide: SlideView;
  saved: SlideView | undefined;
  slideCount: number;
  editable: boolean;
  dirty: boolean;
  reference: ReactNode;
  onText(id: string, expect: string, text: string): void;
  onMove(offset: number): void;
  onRestructure(operations: unknown[]): void;
}) {
  const [confirmingRemoval, setConfirmingRemoval] = useState(false);
  const structural = editable && !dirty;
  return (
    <RecordGroup>
      <Metadata>{t("slides.select")}</Metadata>
      {slide.notes.map((run, index) => {
        const savedText =
          saved?.notes.find((entry) => entry.id === run.id)?.text ?? run.text;
        return (
          <RunField
            key={run.id}
            label={t("slides.notes", { number: index + 1 })}
            text={run.text}
            editable={editable}
            onCommit={(text) => onText(run.id, savedText, text)}
          />
        );
      })}
      <Toolbar label={t("slides.slideActions")}>
        <Button
          disabled={!structural || slide.number === 1}
          onClick={() => onMove(-1)}
        >
          {t("slides.moveUp")}
        </Button>
        <Button
          disabled={!structural || slide.number === slideCount}
          onClick={() => onMove(1)}
        >
          {t("slides.moveDown")}
        </Button>
        <Button
          disabled={!structural}
          onClick={() =>
            onRestructure([{ type: "duplicateSlide", id: slide.id }])
          }
        >
          {t("slides.duplicate")}
        </Button>
        <Button
          disabled={!structural || slideCount < 2}
          onClick={() => setConfirmingRemoval(true)}
        >
          {t("slides.deleteSlide")}
        </Button>
        {reference}
      </Toolbar>
      {dirty && <Metadata>{t("slides.structureBlocked")}</Metadata>}
      {confirmingRemoval && (
        <RecordGroup>
          <Callout kind="warning">
            {t("slides.deleteSlide.confirm", { number: slide.number })}
          </Callout>
          <FormActions label={t("slides.slideActions")}>
            <Button onClick={() => setConfirmingRemoval(false)}>
              {t("cancel")}
            </Button>
            <Button
              disabled={!structural}
              onClick={() => {
                setConfirmingRemoval(false);
                onRestructure([{ type: "deleteSlide", id: slide.id }]);
              }}
            >
              {t("slides.deleteSlide")}
            </Button>
          </FormActions>
        </RecordGroup>
      )}
    </RecordGroup>
  );
}
