export interface PixelSize {
  width: number;
  height: number;
}

const HEADER_CHARS = 4096;

function headerBytes(base64: string): Uint8Array | null {
  const slice = base64.slice(0, HEADER_CHARS - (HEADER_CHARS % 4));
  try {
    const binary = atob(slice);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
    return bytes;
  } catch {
    return null;
  }
}

const be32 = (b: Uint8Array, at: number) =>
  (b[at]! << 24) | (b[at + 1]! << 16) | (b[at + 2]! << 8) | b[at + 3]!;
const be16 = (b: Uint8Array, at: number) => (b[at]! << 8) | b[at + 1]!;
const le16 = (b: Uint8Array, at: number) => b[at]! | (b[at + 1]! << 8);
const le32 = (b: Uint8Array, at: number) =>
  b[at]! | (b[at + 1]! << 8) | (b[at + 2]! << 16) | (b[at + 3]! << 24);

const starts = (b: Uint8Array, at: number, signature: readonly number[]) =>
  signature.every((byte, i) => b[at + i] === byte);

function pngSize(b: Uint8Array): PixelSize | null {
  if (!starts(b, 0, [0x89, 0x50, 0x4e, 0x47])) return null;
  if (b.length < 24) return null;
  return { width: be32(b, 16), height: be32(b, 20) };
}

function gifSize(b: Uint8Array): PixelSize | null {
  if (!starts(b, 0, [0x47, 0x49, 0x46])) return null;
  if (b.length < 10) return null;
  return { width: le16(b, 6), height: le16(b, 8) };
}

function jpegSize(b: Uint8Array): PixelSize | null {
  if (!starts(b, 0, [0xff, 0xd8])) return null;
  let at = 2;
  while (at + 9 < b.length) {
    if (b[at] !== 0xff) return null;
    const marker = b[at + 1]!;
    if (marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc) {
      return { height: be16(b, at + 5), width: be16(b, at + 7) };
    }
    if (marker === 0xd8 || (marker >= 0xd0 && marker <= 0xd9)) {
      at += 2;
      continue;
    }
    at += 2 + be16(b, at + 2);
  }
  return null;
}

function webpSize(b: Uint8Array): PixelSize | null {
  if (!starts(b, 0, [0x52, 0x49, 0x46, 0x46]) || !starts(b, 8, [0x57, 0x45, 0x42, 0x50])) {
    return null;
  }
  if (starts(b, 12, [0x56, 0x50, 0x38, 0x20]) && b.length >= 30) {
    return { width: le16(b, 26) & 0x3fff, height: le16(b, 28) & 0x3fff };
  }
  if (starts(b, 12, [0x56, 0x50, 0x38, 0x4c]) && b.length >= 25) {
    const bits = le32(b, 21);
    return { width: (bits & 0x3fff) + 1, height: ((bits >> 14) & 0x3fff) + 1 };
  }
  if (starts(b, 12, [0x56, 0x50, 0x38, 0x58]) && b.length >= 30) {
    const w = b[24]! | (b[25]! << 8) | (b[26]! << 16);
    const h = b[27]! | (b[28]! << 8) | (b[29]! << 16);
    return { width: w + 1, height: h + 1 };
  }
  return null;
}

export function imageSizeFromBase64(base64: string): PixelSize | null {
  const bytes = headerBytes(base64);
  if (!bytes) return null;
  const size = pngSize(bytes) ?? jpegSize(bytes) ?? gifSize(bytes) ?? webpSize(bytes);
  if (!size || size.width <= 0 || size.height <= 0) return null;
  return size;
}
