import { type FileSaveResult, type SaveFileRequest } from "@flame/component-sdk";

const INLINE_IMAGE = /^data:image\/(?:avif|gif|jpeg|jpg|png|svg\+xml|webp)(?:;[^,]*)?,/i;

export function isInlineImage(source: string): boolean {
  return INLINE_IMAGE.test(source);
}

export type ImageExport = (source: string) => Promise<FileSaveResult>;

export function createImageExport(
  filenameStem: string,
  saveFile: (request: SaveFileRequest) => Promise<FileSaveResult>,
): ImageExport {
  return async (source) => {
    if (!isInlineImage(source)) throw new TypeError("image export requires an inline image");
    // Image decoding keeps SVG scripts and external resources inert. Only the
    // rasterized PNG crosses the file boundary, never executable source material.
    const image = new Image();
    image.src = source;
    await image.decode();
    const canvas = document.createElement("canvas");
    canvas.width = image.naturalWidth;
    canvas.height = image.naturalHeight;
    const context = canvas.getContext("2d");
    if (!context) throw new Error("image export could not create a canvas");
    context.drawImage(image, 0, 0);
    const blob = await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob((material) => {
        if (material) resolve(material);
        else reject(new Error("image export could not encode PNG material"));
      }, "image/png");
    });
    return saveFile({
      filename: `${filenameStem}.png`,
      mime: "image/png",
      bytes: new Uint8Array(await blob.arrayBuffer()),
    });
  };
}
