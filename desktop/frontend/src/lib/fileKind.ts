import { langFromPath } from "@/lib/highlight/shiki";
import { fileExtension } from "@/lib/path";

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
  const extension = fileExtension(path);
  if (IMAGE_EXTENSIONS.has(extension)) return "image";
  if (DOCUMENT_EXTENSIONS.has(extension)) return "document";
  return langFromPath(path) === "text" ? "file" : "code";
}
