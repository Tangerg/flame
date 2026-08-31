// DISPLAY only. The runtime owns real path semantics; this shortens what it returned.

/** Last segment ("/a/b/c/" → "c"); the input itself when there is nothing to split. */
export function basename(path: string): string {
  return path.replace(/\/+$/, "").split("/").at(-1) || path;
}

/** Split for a two-line row. `directory` is "" when there is no second line to draw. */
export function splitFilePath(path: string): { directory: string; name: string } {
  // Cut and slice the SAME string: measuring on the stripped path and slicing the original
  // returns the separator with the name, so a trailing "/" reads as "project/".
  const trimmed = path.replace(/\/+$/, "");
  const cut = trimmed.lastIndexOf("/");
  if (cut < 0) return { directory: "", name: trimmed || path };
  return { directory: trimmed.slice(0, cut), name: trimmed.slice(cut + 1) };
}
