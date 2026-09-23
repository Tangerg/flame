interface FileRef {
  path: string;
  line: number;
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

const HAS_LETTER = /[A-Za-z]/;

function isFileRef(path: string): boolean {
  if (path.startsWith("//")) return false;
  if (path.includes("/")) return HAS_LETTER.test(path);
  const dot = path.lastIndexOf(".");
  if (dot <= 0 || dot === path.length - 1) return false;
  return FILE_EXT.has(path.slice(dot + 1).toLowerCase());
}

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
