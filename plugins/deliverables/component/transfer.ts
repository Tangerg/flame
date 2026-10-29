import { LIMITS } from "../contracts/limits";
import { METHODS } from "../contracts/methods";
import type { MethodInputs, MethodOutputs } from "./methods.generated";
import type { Asset } from "./types";

export function boundedInteger(value: unknown, maximum: number): number {
  const integer = Number(value);
  if (!Number.isSafeInteger(integer) || integer < 0 || integer > maximum)
    throw new Error("deliverables: transfer integer exceeds its bound");
  return integer;
}

export async function sha256(bytes: Uint8Array<ArrayBuffer>): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return [...new Uint8Array(digest)]
    .map((value) => value.toString(16).padStart(2, "0"))
    .join("");
}

export function encodeChunk(bytes: Uint8Array): string {
  if (!bytes.length || bytes.length > LIMITS.chunkBytes)
    throw new Error("deliverables: upload chunk exceeds its bound");
  let binary = "";
  for (const value of bytes) binary += String.fromCharCode(value);
  return btoa(binary);
}

type Download = (
  input: MethodInputs[typeof METHODS.download],
  signal: AbortSignal,
) => Promise<MethodOutputs[typeof METHODS.download]>;

/** One bounded allocation, contiguous acknowledgements, and the exact native hash. */
export async function downloadAsset(
  download: Download,
  asset: Asset,
  signal: AbortSignal,
  progress?: (received: number, total: number) => void,
): Promise<Uint8Array<ArrayBuffer>> {
  signal.throwIfAborted();
  const size = boundedInteger(asset.size, LIMITS.renderBytes);
  if (!size) throw new Error("deliverables: empty transfer asset");
  const bytes = new Uint8Array(size);
  let offset = 0;
  while (offset < size) {
    signal.throwIfAborted();
    const length = Math.min(LIMITS.chunkBytes, size - offset);
    const chunk = await download({ assetId: asset.id, offset, length }, signal);
    signal.throwIfAborted();
    if (
      chunk.asset.id !== asset.id ||
      chunk.asset.sha256 !== asset.sha256 ||
      chunk.asset.filename !== asset.filename ||
      chunk.asset.mime !== asset.mime ||
      boundedInteger(chunk.asset.size, LIMITS.renderBytes) !== size ||
      boundedInteger(chunk.offset, size) !== offset ||
      chunk.base64.length > Math.ceil(length / 3) * 4 ||
      !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/u.test(
        chunk.base64,
      )
    )
      throw new Error(
        "deliverables: download differs from the captured asset or offset",
      );
    const binary = atob(chunk.base64);
    const next = boundedInteger(chunk.next, size);
    if (
      !binary.length ||
      binary.length > length ||
      next !== offset + binary.length ||
      chunk.done !== (next === size)
    )
      throw new Error(
        "deliverables: download is not a contiguous bounded transfer",
      );
    for (let index = 0; index < binary.length; index++)
      bytes[offset + index] = binary.charCodeAt(index);
    offset = next;
    progress?.(offset, size);
  }
  const actual = await sha256(bytes);
  signal.throwIfAborted();
  if (actual !== asset.sha256)
    throw new Error("deliverables: downloaded bytes fail the captured SHA-256");
  return bytes;
}
