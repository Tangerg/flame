// Built from the DIFF's own file set, not the working tree, so nothing is fetched and
// nothing lazy-loads: the whole shape is known the moment the diff arrives.

import type { WorkspaceFileDiff } from "./workspaceQueries";

/** Carried on BOTH node kinds, so a row never has to walk its children to say what it
 *  is worth. */
export interface ReviewTreeStat {
  added: number;
  removed: number;
  /** Distinct from a pair of zeroes, which would claim the file was touched and changed
   *  nothing. */
  binary: boolean;
}

export interface ReviewTreeFileNode extends ReviewTreeStat {
  kind: "file";
  name: string;
  /** Also the selection id, the React key and the diff list's scroll anchor. */
  path: string;
}

export interface ReviewTreeDirectoryNode extends ReviewTreeStat {
  kind: "directory";
  /** Display name; a compressed chain reads as `parent/child`. */
  name: string;
  path: string;
  children: ReviewTreeNode[];
}

export type ReviewTreeNode = ReviewTreeDirectoryNode | ReviewTreeFileNode;

interface MutableDirectory {
  name: string;
  path: string;
  directories: Map<string, MutableDirectory>;
  files: ReviewTreeFileNode[];
}

/** Binary children contribute no lines but DO taint the total, so the flag rides up
 *  rather than being lost. */
function rollUp(children: readonly ReviewTreeStat[]): ReviewTreeStat {
  let added = 0;
  let removed = 0;
  let binary = false;
  for (const child of children) {
    added += child.added;
    removed += child.removed;
    binary ||= child.binary;
  }
  return { added, removed, binary: binary && added === 0 && removed === 0 };
}

function directory(name: string, path: string): MutableDirectory {
  return { name, path, directories: new Map(), files: [] };
}

// Natural and case-insensitive: `item2` before `item10`, and a capitalised name does not
// sort into its own block ahead of the lowercase ones.
function compareName(left: string, right: string): number {
  return left.localeCompare(right, undefined, { numeric: true, sensitivity: "base" });
}

// A repo-relative diff path is mostly unbranched prefix, so without collapsing the
// navigator spends its first rows saying nothing.
function compress(node: ReviewTreeDirectoryNode): ReviewTreeDirectoryNode {
  let current = node;
  while (current.children.length === 1) {
    const onlyChild = current.children[0];
    if (!onlyChild || onlyChild.kind !== "directory") break;
    current = {
      kind: "directory",
      name: `${current.name}/${onlyChild.name}`,
      path: onlyChild.path,
      children: onlyChild.children,
      added: onlyChild.added,
      removed: onlyChild.removed,
      binary: onlyChild.binary,
    };
  }
  return current;
}

function finalize(node: MutableDirectory): ReviewTreeNode[] {
  const directories: ReviewTreeDirectoryNode[] = [];
  for (const child of node.directories.values()) {
    const children = finalize(child);
    directories.push(
      compress({
        kind: "directory",
        name: child.name,
        path: child.path,
        children,
        ...rollUp(children),
      }),
    );
  }
  // Directories first, then files — the ordering every file explorer uses, so
  // the navigator and the workspace tree read the same way. Both arrays were
  // built right here, so sorting them in place aliases nothing.
  directories.sort((left, right) => compareName(left.name, right.name));
  node.files.sort((left, right) => compareName(left.name, right.name));
  return [...directories, ...node.files];
}

export function buildReviewFileTree(files: readonly WorkspaceFileDiff[]): ReviewTreeNode[] {
  const root = directory("", "");
  for (const file of files) {
    const segments = file.path.split("/").filter((segment) => segment.length > 0);
    const name = segments.at(-1);
    if (name === undefined) continue;
    let parent = root;
    for (const segment of segments.slice(0, -1)) {
      const path = parent.path ? `${parent.path}/${segment}` : segment;
      let child = parent.directories.get(segment);
      if (!child) {
        child = directory(segment, path);
        parent.directories.set(segment, child);
      }
      parent = child;
    }
    // Upsert, never blind push: two entries naming one path put two nodes under one
    // key, and React answers a duplicate key with a warning loop that costs frames
    // (CLAUDE.md §5). The later entry wins, which is what a re-read of the same diff
    // means.
    const existing = parent.files.findIndex((candidate) => candidate.path === file.path);
    const node = {
      kind: "file" as const,
      name,
      path: file.path,
      added: file.added ?? 0,
      removed: file.removed ?? 0,
      binary: file.binary === true,
    };
    if (existing < 0) parent.files.push(node);
    else parent.files[existing] = node;
  }
  return finalize(root);
}

/** Substring match on the whole path, so `views/diff` narrows by directory as
 *  well as by filename. An empty query keeps every file. */
export function filterReviewFiles(
  files: readonly WorkspaceFileDiff[],
  query: string,
): WorkspaceFileDiff[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return [...files];
  return files.filter((file) => file.path.toLowerCase().includes(needle));
}
