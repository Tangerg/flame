import * as stylex from "@stylexjs/stylex";
import type { StyleXStyles } from "@stylexjs/stylex";
import { type AnsiSpan, type AnsiTone, parseAnsi } from "@/lib/ansi";
import { toneInk } from "./tone-ink";
import { vocab } from "./vocabulary";

const styles = stylex.create({
  bold: { fontWeight: 600 },
  dim: { opacity: 0.7 },
  underline: { textDecorationLine: "underline" },
});

const TONE: Record<AnsiTone, StyleXStyles> = {
  negative: toneInk.negative,
  success: toneInk.success,
  warning: toneInk.warning,
  info: toneInk.info,
  accent: toneInk.accent,
  muted: vocab.faint,
};

function spanStyle(span: AnsiSpan) {
  return stylex.props(
    span.tone ? TONE[span.tone] : null,
    span.bold ? styles.bold : null,
    span.dim ? styles.dim : null,
    span.underline ? styles.underline : null,
  );
}

export function AnsiText({ text }: { text: string }) {
  return (
    <>
      {parseAnsi(text).map((span, index) => (
        <span key={index} {...spanStyle(span)}>
          {span.text}
        </span>
      ))}
    </>
  );
}
