import type { StyleXStyles } from "@stylexjs/stylex";
import type { Tone } from "@/lib/tone";
import { vocab } from "./vocabulary";

export const toneInk = {
  neutral: vocab.muted,
  accent: vocab.accent,
  success: vocab.success,
  warning: vocab.warning,
  negative: vocab.negative,
  info: vocab.info,
} as const satisfies Record<Tone, StyleXStyles>;
