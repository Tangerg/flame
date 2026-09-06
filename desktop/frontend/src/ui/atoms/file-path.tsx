import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { color } from "@/styles/tokens.stylex";

const styles = stylex.create({
  root: { display: "flex", minWidth: 0, alignItems: "baseline" },
  // The directory truncates from its own start, so what survives is the end — the part
  // nearest the filename. `dir="rtl"` is what moves the ellipsis to the left edge; the inner
  // span puts the text back in reading order.
  directory: {
    minWidth: 0,
    maxWidth: "max-content",
    flex: 1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    textAlign: "left",
    color: color.fgFaint,
  },
  separator: { flexShrink: 0, color: color.fgFaint },
  // Shrinks but never truncates first: a path without its filename identifies nothing.
  filename: {
    minWidth: 0,
    flexShrink: 1,
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
  },
});

export function FilePath({ path, className }: { path: string; className?: string }) {
  const cut = path.lastIndexOf("/");
  const directory = cut > 0 ? path.slice(0, cut) : "";
  const filename = cut >= 0 ? path.slice(cut + 1) : path;
  const root = stylex.props(styles.root);

  return (
    <span {...root} className={cn(root.className, className)} title={path}>
      {directory !== "" && (
        <span dir="rtl" {...stylex.props(styles.directory)}>
          <span dir="ltr">{directory}</span>
        </span>
      )}
      {cut >= 0 && <span {...stylex.props(styles.separator)}>/</span>}
      <span {...stylex.props(styles.filename)}>{filename}</span>
    </span>
  );
}
