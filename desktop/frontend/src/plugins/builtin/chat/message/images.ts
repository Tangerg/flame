import { definePlugin } from "@/plugins/sdk";
import { configureImageSave, type ImageSave } from "./application/imageSave";

export function createMessageImagesPlugin(saveImage: ImageSave) {
  return definePlugin({
    name: "flame.builtin.message-images",
    setup(ctx) {
      ctx.cleanup(configureImageSave(saveImage));
    },
  });
}
