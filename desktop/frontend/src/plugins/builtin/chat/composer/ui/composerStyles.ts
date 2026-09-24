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
  steerHint: { margin: 0, paddingTop: space.s1, color: color.fgFaint },

  tray: {
    display: "flex",
    flexWrap: "wrap",
    alignItems: "center",
    gap: space.s1_5,
    maxHeight: "calc(var(--spacing) * 32)",
    overflowY: "auto",
    overscrollBehavior: "contain",
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
  thumbOpen: {
    display: "block",
    height: "100%",
    width: "100%",
    borderWidth: 0,
    padding: 0,
    backgroundColor: "transparent",
    cursor: "zoom-in",
  },
  thumbImage: { display: "block", height: "100%", width: "100%", objectFit: "cover" },
  previewImage: {
    display: "block",
    maxHeight: "calc(100vh - 96px)",
    maxWidth: "calc(100vw - 96px)",
    objectFit: "contain",
  },
  thumbRemove: { position: "absolute", top: space.s0_5, right: space.s0_5 },

  dropCue: {
    position: "absolute",
    inset: 0,
    zIndex: 1,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    gap: space.s2,
    borderRadius: "inherit",
    backgroundColor: surface.surface2,
    color: color.fg,
    pointerEvents: "none",
  },
  dropCueRefused: { color: color.fgMuted },
  dropLabel: { fontWeight: weight.medium },
});
