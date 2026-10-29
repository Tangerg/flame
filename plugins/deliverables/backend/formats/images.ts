import type { AuthoredImage } from "../../contracts/content";

export function imageBytes(image: AuthoredImage): Uint8Array {
  if (
    image.base64.length % 4 !== 0 ||
    !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/u.test(
      image.base64,
    )
  )
    throw new Error("image must contain canonical base64 data");
  const bytes = Buffer.from(image.base64, "base64");
  if (bytes.length === 0 || bytes.length > 8 * 1024 * 1024)
    throw new Error("image exceeds the byte limit");
  let width = 0;
  let height = 0;
  if (image.mime === "image/png") {
    if (
      bytes.length < 33 ||
      !bytes
        .subarray(0, 8)
        .equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])) ||
      bytes.toString("ascii", 12, 16) !== "IHDR"
    )
      throw new Error("image does not match PNG format");
    width = bytes.readUInt32BE(16);
    height = bytes.readUInt32BE(20);
  } else {
    if (bytes[0] !== 255 || bytes[1] !== 216)
      throw new Error("image does not match JPEG format");
    let cursor = 2;
    while (cursor + 4 <= bytes.length) {
      if (bytes[cursor] !== 255) throw new Error("invalid JPEG marker");
      while (bytes[cursor] === 255) cursor++;
      const marker = bytes[cursor++]!;
      if (marker === 217 || marker === 218) break;
      if (marker >= 208 && marker <= 215) continue;
      const length = bytes.readUInt16BE(cursor);
      if (length < 2 || cursor + length > bytes.length)
        throw new Error("invalid JPEG segment");
      if (
        [
          192, 193, 194, 195, 197, 198, 199, 201, 202, 203, 205, 206, 207,
        ].includes(marker)
      ) {
        if (length < 7) throw new Error("invalid JPEG size segment");
        height = bytes.readUInt16BE(cursor + 3);
        width = bytes.readUInt16BE(cursor + 5);
        break;
      }
      cursor += length;
    }
  }
  if (
    !width ||
    !height ||
    width > 16384 ||
    height > 16384 ||
    width * height > 40_000_000
  )
    throw new Error("image pixel dimensions exceed the supported limits");
  return bytes;
}
