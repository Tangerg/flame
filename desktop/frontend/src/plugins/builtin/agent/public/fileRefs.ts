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

const TOKEN = /(?<![\w/.@-])([A-Za-z0-9._\-/]+)(?::(\d+))?(?::(\d+))?/g;

/** A run of digits and slashes is arithmetic or a date. */
const HAS_LETTER = /[A-Za-z]/;

function isFileRef(path: string): boolean {
  // What is left of a URL once its scheme has been matched as its own token: `https://host/p`
  // arrives here as `//host/p`, which has a separator and would otherwise qualify. A tool that
  // prints one URL prints several — `git clone`, `npm notice`, a dev server's listen line.
  if (path.startsWith("//")) return false;
  // The separator branch used to accept any alphanumeric, which is the same precision hole the
  // header warns about on the other branch. Measured in ordinary tool output: `rate limit 30/60`
  // linked `30/60`, `ratio was 3/4` linked `3/4`, and `on 2024/01/15` offered to open a date.
  if (path.includes("/")) return HAS_LETTER.test(path);
  const dot = path.lastIndexOf(".");
  if (dot <= 0 || dot === path.length - 1) return false;
  return FILE_EXT.has(path.slice(dot + 1).toLowerCase());
}

/** A text with
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
