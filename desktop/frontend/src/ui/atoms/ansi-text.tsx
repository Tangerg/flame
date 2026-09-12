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

/**
 * What an SGR colour means HERE. Exhaustive against the tone union: a tone added to the parser
 * and not answered here is a compile error, not a span that quietly renders in the ink around it.
 *
 * Five of the six are the product's own tones, so they point at `toneInk` rather than restating
 * it. `muted` is the exception and is NOT `Tone.neutral`: ANSI's dim is "below the text around
 * me", a step under muted, which is why it reads from `vocab` directly.
 */
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
