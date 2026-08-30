import { cn } from "@/lib/classNames";

/**
 * A path truncated from the LEFT so the filename survives — plain `truncate` cuts the
 * end, which is the only part a reader is looking for.
 *
 * The directory is the elastic part and clips at its own left edge via `direction: rtl`;
 * the separator and filename are pinned in their own elements so the reordering cannot
 * reach them.
 */
export function FilePath({ path, className }: { path: string; className?: string }) {
  const cut = path.lastIndexOf("/");
  const directory = cut > 0 ? path.slice(0, cut) : "";
  const filename = cut >= 0 ? path.slice(cut + 1) : path;

  return (
    <span className={cn("flex min-w-0 items-baseline", className)} title={path}>
      {directory !== "" && (
        // `flex-1` (zero basis) grows from nothing rather than shrinking from full size.
        // Shrinking shares the deficit proportionally, so the filename gave up a whole
        // character for a sub-pixel overflow. `max-w-max` caps growth at the directory's
        // own length, or a short path pushes its filename to the far edge.
        <span dir="rtl" className="min-w-0 max-w-max flex-1 truncate text-left text-fg-faint">
          {/* The `rtl` above costs the string's own order unless this isolates it back:
              the leading slash of `/Users/…` is bidi-neutral, resolves to RTL from the
              surrounding paragraph and reorders to the far right, rendering every
              absolute path as `…/application//name`. */}
          <span dir="ltr">{directory}</span>
        </span>
      )}
      {/* Outside the branch above because a root-level path (`/etc`) has no directory but
          the slash is still part of the name. */}
      {cut >= 0 && <span className="shrink-0 text-fg-faint">/</span>}
      {/* `shrink`, not `shrink-0`: pinned against the box, this became the row's
          min-content and pushed the row past its container in a narrow column. */}
      <span className="min-w-0 shrink truncate">{filename}</span>
    </span>
  );
}
