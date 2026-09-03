// Precision over recall: a token qualifies only with a path separator OR a known source-file
// extension, so prose like "e.g." and a version "1.2.3" do not light up.

interface FileRef {
  path: string;
  line: number; // 0 = no specific line
  /** 0 = none. Carried so the rendered reference reads as the tool wrote it —
   *  the viewer navigates by line, but `tsc`, `grep` and `eslint` all emit
   *  `path:line:col`, and a link that silently drops the column is showing the
   *  reader text that was never in the output. */
  column: number;
}

export type RefSegment = string | FileRef;

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

// The lookbehind is what keeps a match from starting mid-token, inside an email or path.
const TOKEN = /(?<![\w/.@-])([A-Za-z0-9._\-/]+)(?::(\d+))?(?::(\d+))?/g;

function isFileRef(path: string): boolean {
  if (path.includes("/") && /[A-Za-z0-9]/.test(path)) return true;
  const dot = path.lastIndexOf(".");
  if (dot <= 0 || dot === path.length - 1) return false;
  return FILE_EXT.has(path.slice(dot + 1).toLowerCase());
}

/** Split `text` into plain strings and FileRef objects, in order. A text with
 *  no references returns a single-element [text] array. */
export function parseFileRefs(text: string): RefSegment[] {
  const out: RefSegment[] = [];
  let last = 0;
  for (const m of text.matchAll(TOKEN)) {
    const path = m[1]!;
    if (!isFileRef(path)) continue;
    const start = m.index;
    if (start > last) out.push(text.slice(last, start));
    out.push({ path, line: m[2] ? Number(m[2]) : 0, column: m[3] ? Number(m[3]) : 0 });
    last = start + m[0].length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out.length > 0 ? out : [text];
}
