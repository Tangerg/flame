import type { Format } from "./content";

export const MIME: Readonly<Record<Format, string>> = Object.freeze({
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
  pdf: "application/pdf",
});

export const ASSET_MIME = Object.freeze({
  pdf: MIME.pdf,
  png: "image/png",
  csv: "text/csv;charset=utf-8",
  zip: "application/zip",
} as const);
