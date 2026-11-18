import * as stylex from "@stylexjs/stylex";
import { Fragment } from "react";
import { cn } from "@/lib/classNames";
import { splitFilePath } from "@/lib/path";
import { vocab } from "@flame/ui";
import { color } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  root: { display: "flex", minWidth: 0, alignItems: "baseline" },
  wrapped: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  directoryInk: { color: color.fgFaint },
  directory: {
    minWidth: 0,
    maxWidth: "max-content",
    flex: 1,
    textAlign: "left",
  },
  separator: { flexShrink: 0 },
  filename: {
    minWidth: 0,
    flexShrink: 1,
  },
});

export function FilePath({
  path,
  className,
  wrap = false,
}: {
  path: string;
  className?: string;
  wrap?: boolean;
}) {
  if (wrap) {
    const segments = path.split("/");
    const filename = segments.pop();
    return (
      <span
        dir="ltr"
        className={cn(stylex.props(styles.wrapped).className, className)}
        title={path}
      >
        {segments.length > 0 && (
          <span {...stylex.props(styles.directoryInk)}>
            {segments.map((segment, index) => (
              <Fragment key={index}>
                {segment}/<wbr />
              </Fragment>
            ))}
          </span>
        )}
        {filename}
      </span>
    );
  }

  const { directory, name } = splitFilePath(path);
  const hasDirectorySeparator = path.includes("/");
  const root = stylex.props(styles.root);

  return (
    <span {...root} className={cn(root.className, className)} title={path}>
      {directory !== "" && (
        <span dir="rtl" {...stylex.props(styles.directory, styles.directoryInk, vocab.truncate)}>
          <span dir="ltr">{directory}</span>
        </span>
      )}
      {hasDirectorySeparator && (
        <span {...stylex.props(styles.separator, styles.directoryInk)}>/</span>
      )}
      <span {...stylex.props(styles.filename, vocab.truncate)}>{name}</span>
    </span>
  );
}
