import * as stylex from "@stylexjs/stylex";
import type { StyleXStyles } from "@stylexjs/stylex";
import type { Tone } from "@/lib/tone";
import { color } from "@/styles/tokens.stylex";

const styles = stylex.create({
  neutral: { color: color.fgMuted },
  accent: { color: color.accent },
  success: { color: color.success },
  warning: { color: color.warning },
  negative: { color: color.negative },
  info: { color: color.info },
});

/**
 * A tone as bare ink, for a word that carries the meaning without a plate under it.
 *
 * `Badge` maps the same vocabulary onto a fill and an ink; this is the same words with no
 * fill — a failed command in a run digest, a span's status, a severity in a log line. Five
 * call sites had each written their own `Record<…, string>` of `"text-negative"` and friends,
 * which is how one of them ended up mapping a domain word straight onto a utility class.
 */
export const toneInk: Record<Tone, StyleXStyles> = {
  neutral: styles.neutral,
  accent: styles.accent,
  success: styles.success,
  warning: styles.warning,
  negative: styles.negative,
  info: styles.info,
};
