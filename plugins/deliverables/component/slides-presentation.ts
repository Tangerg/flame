import { type Translate } from "@flame/component-sdk";
import { type ElementView, type SlideLayout } from "../contracts/content";

export const EMU_PER_INCH = 914400;

export type LayoutRead = { value?: SlideLayout; error?: string };

export function inches(value: number): string {
  return String(Math.round((value / EMU_PER_INCH) * 1000) / 1000);
}

// Runs carry no paragraph boundaries, so joining them would fuse words.
function excerpt(element: ElementView): string {
  const runs = element.text.filter((run) => run.text.trim());
  const text = (runs[0]?.text ?? "").replace(/\s+/gu, " ").trim();
  return text.length > 80 || runs.length > 1 ? `${text.slice(0, 80)}…` : text;
}

export function label(element: ElementView, t: Translate): string {
  return excerpt(element) || element.name || t(`slides.kind.${element.kind}`);
}
