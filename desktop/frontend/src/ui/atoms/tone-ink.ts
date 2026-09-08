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
 * only says which of them a domain word points at. Four tables had each re-declared the
 * ladder (here, the button's tones, the ANSI map, the activity mark), so `color.negative`
 * was written four times and a fifth answer to "what colour is a failure" was one edit away.
 *
 * `satisfies` rather than an annotation, so each entry keeps its own style type — a
 * `Record<Tone, StyleXStyles>` widens them all and loses that at every call site.
 */
export const toneInk = {
  // A neutral word is MUTED, not faint. Faint reads as "ignore me", which is wrong for a
  // glyph whose whole job is to say which tool ran — see `AgentActivityDisclosure`'s mark.
  neutral: vocab.muted,
  accent: vocab.accent,
  success: vocab.success,
  warning: vocab.warning,
  negative: vocab.negative,
  info: vocab.info,
} as const satisfies Record<Tone, StyleXStyles>;
