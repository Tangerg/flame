// Path DISPLAY helpers — how a cwd reads in chrome (chips, tree nodes),
// not filesystem logic. The runtime owns real path semantics (jail,
// normalization); the UI only ever shortens what the server returned.

/** Last segment of a directory path ("/a/b/c/" → "c"); the input itself
 *  when there's nothing to split (root, ""). */
export function basename(path: string): string {
  return path.replace(/\/+$/, "").split("/").at(-1) || path;
}

/**
 * A path split for a two-line row. `directory` is "" for a path with no separator, which is
 * the signal for "no second line" — an empty one reserves the height and says nothing.
 */
export function splitFilePath(path: string): { directory: string; name: string } {
  // Cut and slice the SAME string. Measuring the cut on the trailing-slash-stripped
  // path and then slicing the original returned the separator with the name, so a
  // cwd ending in "/" read as "project/" in a two-line row while `basename` — which
  // strips first — called the same path "project" in a chip beside it.
  const trimmed = path.replace(/\/+$/, "");
  const cut = trimmed.lastIndexOf("/");
  if (cut < 0) return { directory: "", name: trimmed || path };
  return { directory: trimmed.slice(0, cut), name: trimmed.slice(cut + 1) };
}
