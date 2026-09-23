import * as stylex from "@stylexjs/stylex";
import { color, space, surface, weight } from "@/styles/tokens.stylex";

export const composerStyles = stylex.create({
  editorInset: {
    paddingTop: "var(--composer-editor-top)",
    paddingRight: "var(--composer-editor-end)",
    paddingBottom: "var(--composer-editor-bottom)",
    paddingLeft: "var(--composer-editor-start)",
  },
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
});
