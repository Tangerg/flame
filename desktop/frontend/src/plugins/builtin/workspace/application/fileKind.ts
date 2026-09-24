import { langFromPath } from "@/lib/highlight/shiki";
import { workspaceErrorClassifier } from "./ports/workspaceErrorClassifier";

export type FileKind = "image" | "document" | "code" | "file";

const IMAGE_EXTENSIONS = new Set([
  "apng",
  "avif",
  "bmp",
  "gif",
  "heic",
  "ico",
  "jpeg",
  "jpg",
  "png",
  "svg",
  "tif",
  "tiff",
  "webp",
]);

const DOCUMENT_EXTENSIONS = new Set(["adoc", "markdown", "md", "mdx", "rst", "txt"]);

export function fileKind(path: string): FileKind {
  const name = path.slice(path.lastIndexOf("/") + 1);
  const dot = name.lastIndexOf(".");
  const extension = dot > 0 ? name.slice(dot + 1).toLowerCase() : "";
  if (IMAGE_EXTENSIONS.has(extension)) return "image";
  if (DOCUMENT_EXTENSIONS.has(extension)) return "document";
  return langFromPath(path) === "text" ? "file" : "code";
}

export function isUnsupportedFileRead(error: unknown): boolean {
  return workspaceErrorClassifier().isUnsupportedFile(error);
}
