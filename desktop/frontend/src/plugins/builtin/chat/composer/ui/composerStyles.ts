import * as stylex from "@stylexjs/stylex";
import { color, space, surface, weight } from "@/styles/tokens.stylex";

/** The material — the glass, the edge, the corner — belongs to
 *  `AgentComposerSurface`; what is here is where the editor sits inside it. */
export const composerStyles = stylex.create({
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
  toolbarSpacer: { minWidth: space.s2, flex: 1 },

  attachmentRow: {
    display: "flex",
    flexWrap: "wrap",
    paddingTop: space.s1,
    paddingBottom: space.s1,
  },
  thumb: {
    position: "relative",
    height: space.s14,
    width: space.s14,
    overflow: "hidden",
    borderRadius: "var(--composer-attachment-radius)",
  },
  thumbImage: { height: "100%", width: "100%", objectFit: "cover" },
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
  dropTarget: {
    animation: "var(--animate-rise-in)",
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    gap: space.s3,
    borderRadius: "var(--shape-composer)",
    borderWidth: "2px",
    borderStyle: "dashed",
    borderColor: surface.fieldStrong,
    backgroundColor: surface.canvas,
    paddingInline: space.s14,
    paddingBlock: space.s12,
    boxShadow: "var(--shadow-modal)",
  },
  dropLabel: { fontWeight: weight.medium, color: color.fgSoft },
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
