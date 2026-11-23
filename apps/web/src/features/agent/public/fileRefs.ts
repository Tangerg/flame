export interface FileRef {
  path: string;
  line: number;
  column: number;
}

type RefSegment = string | FileRef;

const FILE_EXT = new Set([
  "ts",
  "tsx",
  "js",
  "jsx",
  "mjs",
  "cjs",
  "go",
  "py",
  "rs",
  "java",
  "kt",
  "kts",
  "c",
  "cc",
  "cpp",
  "cxx",
  "h",
  "hpp",
  "cs",
  "rb",
  "php",
  "swift",
  "scala",
  "sh",
  "bash",
  "zsh",
  "md",
  "mdx",
  "json",
  "jsonc",
  "yaml",
  "yml",
  "toml",
  "ini",
  "env",
  "sql",
  "html",
  "htm",
  "css",
  "scss",
  "sass",
  "less",
  "vue",
  "svelte",
  "txt",
  "log",
  "proto",
  "gradle",
  "xml",
  "gql",
  "graphql",
  "tf",
  "lua",
  "dart",
  "ex",
  "exs",
  "clj",
  "hs",
  "ml",
  "pl",
  "r",
  "mod",
  "sum",
  "lock",
  "cfg",
  "conf",
]);

const FILE_PATH_CHARACTERS = String.raw`\p{L}\p{M}\p{N}._/-`;

const TOKEN = new RegExp(
  String.raw`(?<![@${FILE_PATH_CHARACTERS}])([${FILE_PATH_CHARACTERS}]+)(?::(\d+))?(?::(\d+))?`,
  "gu",
);

const INLINE_ADDRESS = new RegExp(
  String.raw`^([${FILE_PATH_CHARACTERS}]+):([0-9]+)(?::([0-9]+)|-([0-9]+))?$`,
  "u",
);

const WORKSPACE_FILE_HREF = /^([^?#]+)#L([0-9]+)(?:-L([0-9]+))?$/;

const HAS_LETTER = /\p{L}/u;

const ROOTED = /^\.{0,2}\//;

function hasFileExtension(name: string): boolean {
  const dot = name.lastIndexOf(".");
  if (dot <= 0 || dot === name.length - 1) return false;
  return FILE_EXT.has(name.slice(dot + 1).toLowerCase());
}

function isFileRef(path: string): boolean {
  if (path.startsWith("//")) return false;
  if (!path.includes("/")) return hasFileExtension(path);
  if (!HAS_LETTER.test(path)) return false;
  const segments = path.split("/").filter(Boolean);
  return ROOTED.test(path) || segments.length >= 3 || hasFileExtension(segments.at(-1) ?? "");
}

export function parseFileRefs(text: string): RefSegment[] {
  const out: RefSegment[] = [];
  let last = 0;
  for (const m of text.matchAll(TOKEN)) {
    const rawPath = m[1]!;
    const path = m[2] || /\/\.{1,2}$/.test(rawPath) ? rawPath : rawPath.replace(/\.+$/, "");
    if (!isFileRef(path)) continue;
    const start = m.index;
    if (start > last) out.push(text.slice(last, start));
    out.push({ path, line: m[2] ? Number(m[2]) : 0, column: m[3] ? Number(m[3]) : 0 });
    last = start + m[0].length - (rawPath.length - path.length);
  }
  if (last < text.length) out.push(text.slice(last));
  return out.length > 0 ? out : [text];
}

function isPositiveFilePosition(value: number): boolean {
  return Number.isSafeInteger(value) && value > 0;
}

export function parseInlineFileRef(value: string): FileRef | null {
  const match = INLINE_ADDRESS.exec(value);
  if (!match || match[0] !== value) return null;
  const [, path, lineText, columnText, endText] = match;
  if (!path || !isFileRef(path)) return null;

  const line = Number(lineText);
  const column = columnText === undefined ? 0 : Number(columnText);
  if (
    !isPositiveFilePosition(line) ||
    (columnText !== undefined && !isPositiveFilePosition(column))
  )
    return null;
  if (endText !== undefined) {
    const endLine = Number(endText);
    if (!isPositiveFilePosition(endLine) || endLine < line) return null;
  }
  return { path, line, column };
}

export function parseWorkspaceFileHref(href: string): FileRef | null {
  const match = WORKSPACE_FILE_HREF.exec(href);
  if (!match || match[0] !== href) return null;
  const [, encodedPath, first, last] = match;
  if (!encodedPath) return null;

  let pathname: string;
  try {
    pathname = decodeURI(encodedPath);
  } catch (error) {
    if (error instanceof URIError) return null;
    throw error;
  }
  const path = pathname.startsWith("./") ? pathname.slice(2) : pathname;
  if (!path || path.startsWith("/") || path.split("/").includes("..")) return null;

  const ref = parseInlineFileRef(`${pathname}:${first}${last === undefined ? "" : `-${last}`}`);
  return ref?.path === pathname ? { ...ref, path } : null;
}

// Prose references omit unspecified positions. A quoted range has explicit endpoints.
export function formatFileRef(ref: FileRef, endLine?: number): string {
  if (ref.line <= 0 && endLine === undefined) return ref.path;
  const suffix =
    endLine !== undefined && endLine !== ref.line
      ? `-${endLine}`
      : ref.column > 0
        ? `:${ref.column}`
        : "";
  return `${ref.path}:${ref.line}${suffix}`;
}
