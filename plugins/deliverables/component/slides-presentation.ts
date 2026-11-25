import { type Translate } from "@flame/component-sdk";
import type { ChartData, Frame } from "../contracts/content";
import type { ElementView, SlideLayout, SlideView } from "../contracts/layouts";

export const EMU_PER_INCH = 914400;

export function inches(value: number): string {
  return String(Math.round((value / EMU_PER_INCH) * 1000) / 1000);
}

export const frameText = (frame: Frame) =>
  `${inches(frame.x)}, ${inches(frame.y)} · ${inches(frame.width)} × ${inches(frame.height)} in`;

export const chartText = (data: ChartData) =>
  data.series
    .map(
      (series) =>
        `${series.name}: ${data.categories
          .map(
            (category, point) => `${category} ${series.values[point] ?? "–"}`,
          )
          .join(" · ")}`,
    )
    .join("; ");

// Runs carry no paragraph boundaries, so joining them would fuse words.
function excerpt(element: ElementView): string {
  const runs = element.text.filter((run) => run.text.trim());
  const text = (runs[0]?.text ?? "").replace(/\s+/gu, " ").trim();
  return text.length > 80 || runs.length > 1 ? `${text.slice(0, 80)}…` : text;
}

export function label(element: ElementView, t: Translate): string {
  return excerpt(element) || element.name || t(`slides.kind.${element.kind}`);
}

// An element identity names its slide part before "#element:".
export function slideTarget(object: string): {
  slide: string;
  element?: string;
} {
  const separator = object.indexOf("#element:");
  return separator < 0
    ? { slide: object }
    : { slide: object.slice(0, separator), element: object };
}

const slideName = (slide: SlideView, t: Translate) =>
  [
    t("slides.slide", { number: slide.number }),
    ...(slide.name ? [slide.name] : []),
  ].join(" · ");

// A comment on the selection quotes the element's text, or the slide's name.
export function slideComment(
  slide: SlideView,
  elementId: string | undefined,
  t: Translate,
) {
  const name = slideName(slide, t);
  const element = slide.elements.find((entry) => entry.id === elementId);
  return element
    ? {
        object: element.id,
        quote: element.text.map((run) => run.text).join(""),
        label: `${name} · ${label(element, t)}`,
      }
    : { object: slide.id, quote: slide.name, label: name };
}

// Slide and element identities survive revisions, so an anchored object is
// named wherever the shown revision still has it.
export function describeSlideObject(
  layout: SlideLayout,
  object: string,
  t: Translate,
): string | undefined {
  const target = slideTarget(object);
  const slide = layout.slides.find((entry) => entry.id === target.slide);
  if (!slide || !target.element) return slide && slideName(slide, t);
  const element = slide.elements.find((entry) => entry.id === target.element);
  return element && `${slideName(slide, t)} · ${label(element, t)}`;
}
