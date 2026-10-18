import * as stylex from "@stylexjs/stylex";
import { ARTIFACT_SURFACE } from "@flame/ui";

const transcript = stylex.create({
  margin: { marginBlock: "calc(var(--prose-space) * 5)" },
});

export const artifactFrame = {
  frame: [ARTIFACT_SURFACE.face, transcript.margin],
} as const;
