import { inflateSync } from "node:zlib";
import { decode as decodeJPEG } from "jpeg-js";
import { PNG } from "pngjs";
import type { AuthoredImage } from "../../contracts/content";

const maximumImageBytes = 8 * 1024 * 1024;
const maximumDimension = 16384;
const maximumPixels = 40_000_000;
const pngSignature = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);

function admitDimensions(width: number, height: number): void {
  if (
    !width ||
    !height ||
    width > maximumDimension ||
    height > maximumDimension ||
    width * height > maximumPixels
  )
    throw new Error("image pixel dimensions exceed the supported limits");
}

function decodePNG(bytes: Buffer): void {
  if (
    bytes.length < 33 ||
    !bytes.subarray(0, 8).equals(pngSignature) ||
    bytes.readUInt32BE(8) !== 13 ||
    bytes.toString("latin1", 12, 16) !== "IHDR"
  )
    throw new Error("image does not match PNG format");
  const width = bytes.readUInt32BE(16);
  const height = bytes.readUInt32BE(20);
  admitDimensions(width, height);
  // pngjs bounds ordinary scanlines internally. Its Adam7 path inflates the
  // entire stream, so bound the same compressed payload before invoking it.
  const interlaced = bytes[28] === 1;
  const compressed: Buffer[] = [];
  let ended = false;
  for (let offset = 33; offset < bytes.length;) {
    if (offset + 12 > bytes.length) throw new Error("truncated PNG chunk");
    const length = bytes.readUInt32BE(offset);
    const type = bytes.toString("latin1", offset + 4, offset + 8);
    const end = offset + length + 12;
    if (end > bytes.length || type === "IHDR")
      throw new Error("invalid PNG chunk");
    if (type === "IDAT" && interlaced)
      compressed.push(bytes.subarray(offset + 8, end - 4));
    if (type === "IEND") {
      if (length !== 0 || end !== bytes.length)
        throw new Error("invalid PNG end");
      ended = true;
    }
    offset = end;
  }
  if (!ended) throw new Error("missing PNG end");
  if (interlaced) {
    // At most eight sample bytes per pixel and one filter byte per nonempty
    // pass row; each such row contains at least one of the image's pixels.
    inflateSync(Buffer.concat(compressed), {
      maxOutputLength: width * height * 9,
    });
  }
  PNG.sync.read(bytes, { checkCRC: true });
}

export function imageBytes(image: AuthoredImage): Uint8Array {
  const bytes = Buffer.from(image.base64, "base64");
  if (bytes.length === 0 || bytes.length > maximumImageBytes)
    throw new Error("image exceeds the byte limit");
  if (bytes.toString("base64") !== image.base64)
    throw new Error("image must contain canonical base64 data");
  if (image.mime === "image/png") {
    decodePNG(bytes);
  } else {
    const { width, height } = decodeJPEG(bytes, {
      tolerantDecoding: false,
      maxResolutionInMP: maximumPixels / 1_000_000,
      maxMemoryUsageInMB: 384,
    });
    admitDimensions(width, height);
  }
  return bytes;
}
