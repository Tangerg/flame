import { afterEach, describe, expect, it, vi } from "vitest";
import { createImageExport, isInlineImage } from "./imageExport";

afterEach(() => vi.restoreAllMocks());

describe("image export", () => {
  it.each(["https://remote.test/image.png", "data:text/html,<script></script>", "blob:remote"])(
    "refuses %s before decoding or saving",
    async (source) => {
      const decode = vi.spyOn(HTMLImageElement.prototype, "decode");
      const save = vi.fn();
      expect(isInlineImage(source)).toBe(false);
      await expect(createImageExport("image", save)(source)).rejects.toThrow("inline image");
      expect(decode).not.toHaveBeenCalled();
      expect(save).not.toHaveBeenCalled();
    },
  );

  it("does not save material that the image decoder rejects", async () => {
    const failure = new Error("invalid image bytes");
    vi.spyOn(HTMLImageElement.prototype, "decode").mockRejectedValue(failure);
    const save = vi.fn();
    await expect(createImageExport("image", save)("data:image/png;base64,YmFk")).rejects.toBe(
      failure,
    );
    expect(save).not.toHaveBeenCalled();
  });

  it("reports an unavailable drawing context without saving", async () => {
    vi.spyOn(HTMLImageElement.prototype, "decode").mockResolvedValue();
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
    const save = vi.fn();
    await expect(createImageExport("image", save)("data:image/png;base64,YmFk")).rejects.toThrow(
      "create a canvas",
    );
    expect(save).not.toHaveBeenCalled();
  });

  it.each(["saved", "cancelled", "requested"] as const)(
    "preserves the file owner's %s outcome after PNG encoding",
    async (outcome) => {
      vi.spyOn(HTMLImageElement.prototype, "decode").mockResolvedValue();
      vi.spyOn(HTMLImageElement.prototype, "naturalWidth", "get").mockReturnValue(2);
      vi.spyOn(HTMLImageElement.prototype, "naturalHeight", "get").mockReturnValue(3);
      const draw = vi.fn();
      vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
        drawImage: draw,
      } as unknown as CanvasRenderingContext2D);
      const bytes = new Uint8Array([137, 80, 78, 71]);
      const encode = vi
        .spyOn(HTMLCanvasElement.prototype, "toBlob")
        .mockImplementation((receive) => receive(new Blob([bytes], { type: "image/png" })));
      const save = vi.fn().mockResolvedValue(outcome);
      await expect(createImageExport("image", save)("data:image/png;base64,YmFk")).resolves.toBe(
        outcome,
      );
      expect(draw).toHaveBeenCalledOnce();
      expect(encode).toHaveBeenCalledWith(expect.any(Function), "image/png");
      expect(save).toHaveBeenCalledExactlyOnceWith({
        filename: "image.png",
        mime: "image/png",
        bytes,
      });
    },
  );

  it("propagates a PNG encoder failure without opening the file boundary", async () => {
    vi.spyOn(HTMLImageElement.prototype, "decode").mockResolvedValue();
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      drawImage() {},
    } as unknown as CanvasRenderingContext2D);
    vi.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation((receive) => receive(null));
    const save = vi.fn();
    await expect(createImageExport("image", save)("data:image/png;base64,YmFk")).rejects.toThrow(
      "encode PNG material",
    );
    expect(save).not.toHaveBeenCalled();
  });
});
