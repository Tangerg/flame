import * as stylex from "@stylexjs/stylex";
import { color, space, surface, weight } from "@/styles/tokens.stylex";

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

  // Three rows of attachments sit above the editor, and they had three rhythms between them:
  // gap-2/pb-1, gap-1.5/pb-1, gap-1.5/pb-0.5. The gap difference is real — a thumbnail needs
  // more air than a chip — and the block inset was drift, so the rows agree on it now.
  attachmentRow: {
    display: "flex",
    flexWrap: "wrap",
    paddingTop: space.s1,
    paddingBottom: space.s1,
  },
  /** A staged image is a 56px plate, cropped to fill and wearing the media edge. */
  thumb: {
    position: "relative",
    height: space.s14,
    width: space.s14,
    overflow: "hidden",
    borderRadius: "var(--composer-attachment-radius)",
  },
  thumbImage: { height: "100%", width: "100%", objectFit: "cover" },
  /** The remove control tucks into the plate's own corner rather than beside it. */
  thumbRemove: { position: "absolute", top: space.s0_5, right: space.s0_5 },

  // The drop target covers the window rather than the composer: a file dragged anywhere over
  // the app is meant for the message being written, so the whole window is the target.
  dropScrim: {
    position: "fixed",
    inset: 0,
    zIndex: "var(--layer-modal)",
    display: "grid",
    placeItems: "center",
    backgroundColor: surface.scrim,
    padding: space.s10,
  },
  // A dashed edge, which is the one place in this design a border is drawn to say "not yet" —
  // everywhere else an edge means a real boundary.
  dropTarget: {
    animation: "var(--animate-rise-in)",
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    gap: space.s3,
    borderRadius: "var(--radius-composer)",
    borderWidth: "2px",
    borderStyle: "dashed",
    borderColor: surface.fieldStrong,
    backgroundColor: surface.canvas,
    paddingInline: space.s14,
    paddingBlock: space.s12,
    boxShadow: "var(--shadow-modal)",
  },
  dropLabel: { fontWeight: weight.medium, color: color.fgSoft },
  /** A model's provider under its name: the name is what is chosen, this only disambiguates. */
  modelHint: {
    display: "block",
    minWidth: 0,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    fontWeight: weight.regular,
    color: color.fgFaint,
  },
});
