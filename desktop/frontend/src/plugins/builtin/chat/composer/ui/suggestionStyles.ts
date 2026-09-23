import * as stylex from "@stylexjs/stylex";
import { color, space, weight } from "@/styles/tokens.stylex";

/**
 * The two suggestion panels above the composer: `@` for a file, `/` for a command.
 *
 * They are one shape. The heading inset lives here because `SectionLabel` deliberately does not
 * own it: how deep a heading sits belongs to its container.
 */
export const suggestionStyles = stylex.create({
  panel: { padding: space.s1 },
  heading: { paddingInline: space.s2_5, paddingTop: space.s1_5, paddingBottom: space.s1 },
  // A slash command reads as something typed, so it keeps the monospace face and the accent
  // that says "this is the token". The UA styles a `<code>` brings are not wanted with it.
  command: {
    borderWidth: 0,
    backgroundColor: "transparent",
    padding: 0,
    fontFamily: "var(--font-mono)",
    fontWeight: weight.medium,
    color: color.accent,
  },
  directory: { color: color.fgFaint },
  name: { fontWeight: weight.medium, color: color.fg },
});
