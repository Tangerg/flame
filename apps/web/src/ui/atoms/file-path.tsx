import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import { splitFilePath } from "@/lib/path";
import { vocab } from "@flame/ui";
import { color } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  root: { display: "flex", minWidth: 0, alignItems: "baseline" },
  directory: {
    minWidth: 0,
    maxWidth: "max-content",
    flex: 1,
    textAlign: "left",
    color: color.fgFaint,
  },
  separator: { flexShrink: 0, color: color.fgFaint },
  filename: {
    minWidth: 0,
    flexShrink: 1,
  },
});

export function FilePath({ path, className }: { path: string; className?: string }) {
  const { directory, name } = splitFilePath(path);
  const hasDirectorySeparator = path.includes("/");
  const root = stylex.props(styles.root);

  return (
    <span {...root} className={cn(root.className, className)} title={path}>
      {directory !== "" && (
        <span dir="rtl" {...stylex.props(styles.directory, vocab.truncate)}>
          <span dir="ltr">{directory}</span>
        </span>
      )}
      {hasDirectorySeparator && <span {...stylex.props(styles.separator)}>/</span>}
      <span {...stylex.props(styles.filename, vocab.truncate)}>{name}</span>
    </span>
  );
}
