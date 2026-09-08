import * as stylex from "@stylexjs/stylex";
import { space } from "@/styles/tokens.stylex";

/** The composer's own arrangement. The material — the glass, the edge, the corner — belongs to
 *  `AgentComposerSurface`; what is here is where the editor sits inside it. */
export const composerStyles = stylex.create({
  // Four sides, four density variables: the editor's inset is asymmetric because the send
  // control sits in the footer, not beside the text.
  editorInset: {
    paddingTop: "var(--density-composer-editor-top)",
    paddingRight: "var(--density-composer-editor-end)",
    paddingBottom: "var(--density-composer-editor-bottom)",
    paddingLeft: "var(--density-composer-editor-start)",
  },
  // `lh` so the bounds are a number of LINES: the editor grows from one line to six of whatever
  // size and leading the reader has chosen, rather than to a pixel height that means six lines
  // at one setting and four at another.
  editor: { minHeight: "1.5lh", maxHeight: "6lh" },
  /** Holds the two toolbar ends apart, and yields before either of them does. */
  toolbarSpacer: { minWidth: space.s2, flex: 1 },
});
