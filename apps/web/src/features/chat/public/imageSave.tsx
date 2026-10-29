import { createContext, useContext, type ReactNode } from "react";
import { type ImageExport } from "@/foundation/imageExport";
const ImageSaveContext = createContext<ImageExport | null>(null);
export function ImageSaveProvider({
  saveImage,
  children,
}: {
  saveImage: ImageExport;
  children: ReactNode;
}) {
  return <ImageSaveContext value={saveImage}>{children}</ImageSaveContext>;
}
export function useImageSave(): ImageExport {
  const save = useContext(ImageSaveContext);
  if (!save) throw new Error("ImageSaveProvider is missing");
  return save;
}
