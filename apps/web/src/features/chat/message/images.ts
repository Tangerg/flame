import { definePlugin } from "@/extensions/definePlugin";
import { configureImageSave, type ImageSave } from "@/features/chat/message/application/imageSave";
export function createMessageImagesPlugin(saveImage: ImageSave) {
  return definePlugin({
    name: "flame.builtin.message-images",
    setup(ctx) {
      ctx.cleanup(configureImageSave(saveImage));
    },
  });
}
