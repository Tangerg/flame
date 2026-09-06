import * as stylex from "@stylexjs/stylex";
import { type AnsiSpan, type AnsiTone, parseAnsi } from "@/lib/ansi";
import { color } from "@/styles/tokens.stylex";

/** What an SGR colour means HERE. The parser answers in tones so this stays the one place a
 *  tone becomes a token — two surfaces render command output, and a second copy of this map is
 *  a second answer to "what colour is a failure". */
const styles = stylex.create({
  negative: { color: color.negative },
  success: { color: color.success },
  warning: { color: color.warning },
  info: { color: color.info },
  accent: { color: color.accent },
  muted: { color: color.fgFaint },
  bold: { fontWeight: 600 },
  dim: { opacity: 0.7 },
  underline: { textDecorationLine: "underline" },
});

/** Exhaustive against the tone union: a tone added to the parser and not answered here is a
 *  compile error, not a span that quietly renders in the surrounding ink. */
const TONE: Record<AnsiTone, (typeof styles)[keyof typeof styles]> = {
  negative: styles.negative,
  success: styles.success,
  warning: styles.warning,
  info: styles.info,
  accent: styles.accent,
  muted: styles.muted,
};

function spanStyle(span: AnsiSpan) {
  return stylex.props(
    span.tone ? TONE[span.tone] : null,
    span.bold ? styles.bold : null,
    span.dim ? styles.dim : null,
    span.underline ? styles.underline : null,
  );
}

/** One line of terminal output, with its escape codes read as tone rather than printed. */
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
