export function basename(path: string): string {
  return path.replace(/\/+$/, "").split("/").at(-1) || path;
}

export function splitFilePath(path: string): { directory: string; name: string } {
  const trimmed = path.replace(/\/+$/, "");
  const cut = trimmed.lastIndexOf("/");
  if (cut < 0) return { directory: "", name: trimmed || path };
  return { directory: trimmed.slice(0, cut), name: trimmed.slice(cut + 1) };
}
