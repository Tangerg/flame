import { constants } from "node:fs";
import { lstat, mkdir, open, realpath } from "node:fs/promises";
import { createHash } from "node:crypto";
import { LIMITS } from "../contracts/limits";
import type { Check } from "../contracts/evidence";
import { hasErrorCode } from "./engine-installation";
import { refusal } from "./refusal";

export interface RenderFile {
  filename: string;
  mime: string;
  bytes: Uint8Array;
}

export async function privateDirectory(path: string): Promise<void> {
  await mkdir(path, { mode: 0o700 });
}

export async function checkedDirectory(path: string): Promise<void> {
  const info = await lstat(path);
  if (
    !info.isDirectory() ||
    info.isSymbolicLink() ||
    (await realpath(path)) !== path ||
    (info.mode & 0o077) !== 0 ||
    (process.getuid && info.uid !== process.getuid())
  )
    throw refusal(
      "engine_private_directory",
      "The rendering directory must be a canonical private directory owned by this process.",
    );
}

export async function readOutput(
  path: string,
  maximum: number,
  expected: "content" | "maybeEmpty" = "content",
): Promise<Uint8Array> {
  let handle;
  try {
    // A FIFO can block open forever after its writer exits. Refuse special
    // files first, and keep the descriptor nonblocking across the final check.
    const entry = await lstat(path);
    if (!entry.isFile() || entry.isSymbolicLink())
      throw refusal(
        "engine_output_missing",
        "The rendering process did not produce the required regular output file.",
      );
    handle = await open(
      path,
      constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK,
    );
  } catch (cause) {
    if (hasErrorCode(cause, "ENOENT", "ELOOP"))
      throw refusal(
        "engine_output_missing",
        "The rendering process did not produce the required regular output file.",
      );
    throw cause;
  }
  try {
    const info = await handle.stat();
    if (
      !info.isFile() ||
      info.nlink !== 1 ||
      (expected === "content" && info.size === 0) ||
      info.size > maximum
    )
      throw refusal(
        "engine_output_limit",
        "The rendering output is empty, linked, or exceeds its admitted byte limit.",
      );
    const bytes = await handle.readFile();
    if (bytes.byteLength !== info.size || bytes.byteLength > maximum)
      throw refusal(
        "engine_output_changed",
        "The rendering output changed after its process exited.",
      );
    return bytes;
  } finally {
    await handle.close();
  }
}

export function pdf(bytes: Uint8Array): void {
  if (Buffer.from(bytes.subarray(0, 5)).toString("ascii") !== "%PDF-")
    throw refusal(
      "engine_output_invalid",
      "The renderer did not produce a PDF file.",
    );
}

export function png(bytes: Uint8Array): void {
  const value = Buffer.from(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  if (
    value.length < 45 ||
    !value
      .subarray(0, 8)
      .equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])) ||
    value.readUInt32BE(8) !== 13 ||
    value.toString("ascii", 12, 16) !== "IHDR" ||
    value.toString("ascii", value.length - 8, value.length - 4) !== "IEND"
  )
    throw refusal(
      "engine_preview_invalid",
      "The renderer did not produce a complete PNG preview.",
    );
  const width = value.readUInt32BE(16),
    height = value.readUInt32BE(20);
  if (
    !width ||
    !height ||
    width > LIMITS.previewPixels ||
    height > LIMITS.previewPixels
  )
    throw refusal(
      "engine_preview_limit",
      "The page preview exceeds its admitted pixel dimensions.",
    );
}

export function digest(bytes: Uint8Array): string {
  return createHash("sha256").update(bytes).digest("hex");
}

export function pageCount(pdfinfo: string): number {
  const match = /^Pages:\s+([1-9][0-9]*)\s*$/m.exec(pdfinfo);
  const count = match ? Number(match[1]) : NaN;
  if (!Number.isSafeInteger(count) || count < 1 || count > LIMITS.renderPages)
    throw refusal(
      "engine_page_limit",
      `The PDF must contain between 1 and ${LIMITS.renderPages} pages.`,
    );
  return count;
}

export function textChecks(text: string, truncated: boolean): Check[] {
  const checks: Check[] = [];
  if (truncated)
    checks.push({
      code: "text_truncated",
      severity: "warning",
      message:
        "Extracted text exceeds the retained text limit; the page previews remain the complete visual evidence.",
    });
  if (!text.trim())
    checks.push({
      code: "text_empty",
      severity: "warning",
      message:
        "No searchable text was observed; inspect the page previews for image-only or missing content.",
    });
  if (text.includes("\ufffd"))
    checks.push({
      code: "text_replacement_character",
      severity: "warning",
      message:
        "Extracted text contains replacement characters; verify the corresponding glyphs on the page previews.",
    });
  return checks;
}
