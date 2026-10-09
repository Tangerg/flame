import { fileKind, type FileKind } from "@/lib/fileKind";
import { fileExtension } from "@/lib/path";
import type { IconName } from "./icon";

const KIND_ICON: Record<FileKind, IconName> = {
  image: "file-image",
  document: "file-text",
  code: "file-code",
  file: "file",
};

const EXTENSION_ICON: ReadonlyMap<string, IconName> = new Map([
  ["pdf", "file-pdf"],
  ["zip", "file-archive"],
  ["tar", "file-archive"],
  ["gz", "file-archive"],
  ["tgz", "file-archive"],
  ["bz2", "file-archive"],
  ["xz", "file-archive"],
  ["7z", "file-archive"],
  ["rar", "file-archive"],
  ["mp3", "file-music"],
  ["wav", "file-music"],
  ["ogg", "file-music"],
  ["flac", "file-music"],
  ["m4a", "file-music"],
  ["aac", "file-music"],
  ["opus", "file-music"],
  ["mp4", "file-video"],
  ["webm", "file-video"],
  ["mov", "file-video"],
  ["mkv", "file-video"],
  ["avi", "file-video"],
  ["m4v", "file-video"],
  ["csv", "file-spreadsheet"],
  ["tsv", "file-spreadsheet"],
  ["xls", "file-spreadsheet"],
  ["xlsx", "file-spreadsheet"],
  ["ods", "file-spreadsheet"],
  ["json", "file-braces"],
  ["jsonc", "file-braces"],
  ["json5", "file-braces"],
  ["jsonl", "file-braces"],
  ["ndjson", "file-braces"],
]);

export function fileIconName(path: string): IconName {
  return EXTENSION_ICON.get(fileExtension(path)) ?? KIND_ICON[fileKind(path)];
}
