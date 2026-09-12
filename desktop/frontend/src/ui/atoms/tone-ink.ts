import type { StyleXStyles } from "@stylexjs/stylex";
import type { Tone } from "@/lib/tone";
import { vocab } from "./vocabulary";

/**
 * A tone as bare ink, for a word that carries the meaning without a plate under it.
 *
 * `Badge` maps the same vocabulary onto a fill and an ink; this is the same words with no
 * fill — a failed command in a run digest, a span's status, a severity in a log line.
 *
 * It is a PROJECTION and not a declaration: the values live once in `vocab`, and this file
 * only says which of them a domain word points at.
 *
 * `satisfies` rather than an annotation, so each entry keeps its own style type — a
 * `Record<Tone, StyleXStyles>` widens them all and loses that at every call site.
 */
export const toneInk = {
  neutral: vocab.muted,
  accent: vocab.accent,
  success: vocab.success,
  warning: vocab.warning,
  negative: vocab.negative,
  info: vocab.info,
} as const satisfies Record<Tone, StyleXStyles>;
