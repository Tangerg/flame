export type FileChangeStatus = "added" | "deleted" | "modified" | "moved";

export interface UnifiedDiffFile {
  path: string;
  from?: string;
  status: FileChangeStatus;
  added: number;
  removed: number;
}

const DEV_NULL = "/dev/null";

/**
 * Reads the `patch` argument of an `apply_patch` call.
 *
 * The argument is the ONLY description of the change that exists while the call runs — the
 * receipt arrives with `item.completed`. It states an intent, never an outcome: a patch that
 * fails to apply still parses, so callers must not present this as something that happened.
 *
 * Git-compatible unified diff, matching what the tool's own schema promises: create, modify,
 * delete and rename, the last carried by rename metadata rather than by the hunk body.
 */
export function parseUnifiedDiff(patch: string): UnifiedDiffFile[] {
  const files: UnifiedDiffFile[] = [];
  let open: OpenFile | undefined;

  for (const line of patch.split("\n")) {
    if (line.startsWith("diff --git ")) {
      if (open) files.push(...sealed(open));
      open = openedFrom(line);
      continue;
    }
    // A bare `---` pair is a whole file entry on its own: the schema asks for a Git-compatible
    // diff, but a plain unified diff satisfies `patch(1)` and models emit one.
    if (line.startsWith("--- ") && (!open || open.inHunk)) {
      if (open) files.push(...sealed(open));
      open = { added: 0, removed: 0, inHunk: false };
    }
    if (!open) continue;

    if (line.startsWith("--- ")) open.old = headerPath(line.slice(4));
    else if (line.startsWith("+++ ")) open.new = headerPath(line.slice(4));
    else if (line.startsWith("@@")) open.inHunk = true;
    // `+++`/`---` are headers and are matched above, so a hunk body cannot be miscounted.
    else if (open.inHunk && line.startsWith("+")) open.added += 1;
    else if (open.inHunk && line.startsWith("-")) open.removed += 1;
    // Git's extended headers, which precede any hunk and are the only account of a rename:
    // a pure rename carries no hunk at all.
    else if (!open.inHunk) applyExtendedHeader(open, line);
  }
  if (open) files.push(...sealed(open));
  return files;
}

// Matched a word at a time: these are wire tokens of the diff format, and spelling them as
// sentences reads as user-facing copy to anything scanning this ring for untranslated text.
const RENAME = "rename ";
const CREATE = "new ";
const DELETE = "deleted ";
const FROM = "from ";
const TO = "to ";

function applyExtendedHeader(open: OpenFile, line: string): void {
  if (line.startsWith(RENAME)) {
    const rest = line.slice(RENAME.length);
    if (rest.startsWith(FROM)) open.old = trimmedPath(rest.slice(FROM.length));
    else if (rest.startsWith(TO)) open.new = trimmedPath(rest.slice(TO.length));
    return;
  }
  if (line.startsWith(CREATE)) open.old = undefined;
  else if (line.startsWith(DELETE)) open.new = undefined;
}

interface OpenFile {
  old?: string;
  new?: string;
  added: number;
  removed: number;
  inHunk: boolean;
}

// `diff --git a/x b/y` seeds both names so a rename with no `---`/`+++` pair still resolves.
// Ambiguous when a path contains " b/", which the `---`/`+++` headers then correct.
function openedFrom(line: string): OpenFile {
  const paths = line.slice("diff --git ".length);
  const split = paths.indexOf(" b/");
  const file: OpenFile = { added: 0, removed: 0, inHunk: false };
  if (split > 0) {
    file.old = trimmedPath(stripPrefix(paths.slice(0, split)));
    file.new = trimmedPath(stripPrefix(paths.slice(split + 1)));
  }
  return file;
}

// Nothing to show without a path: a malformed entry drops rather than drawing a blank row
// under a real one.
function sealed(file: OpenFile): UnifiedDiffFile[] {
  const path = file.new ?? file.old;
  if (path === undefined) return [];
  return [
    {
      path,
      status: status(file),
      ...(file.old && file.new && file.old !== file.new ? { from: file.old } : {}),
      added: file.added,
      removed: file.removed,
    },
  ];
}

function status(file: OpenFile): FileChangeStatus {
  if (!file.old) return "added";
  if (!file.new) return "deleted";
  return file.old === file.new ? "modified" : "moved";
}

// A timestamp may follow the path on a `---`/`+++` header, and a tab separates it.
function headerPath(rest: string): string | undefined {
  const path = trimmedPath(stripPrefix(rest.split("\t", 1)[0] ?? ""));
  return path === DEV_NULL ? undefined : path;
}

function stripPrefix(path: string): string {
  const trimmed = path.trim();
  return /^[ab]\//.test(trimmed) ? trimmed.slice(2) : trimmed;
}

function trimmedPath(path: string): string | undefined {
  const trimmed = path.trim();
  return trimmed === "" ? undefined : trimmed;
}
