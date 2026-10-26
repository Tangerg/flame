import { createContext, useContext, type ReactNode } from "react";
export type ImageSave = (source: string) => Promise<boolean>;
const ImageSaveContext = createContext<ImageSave | null>(null);
export function ImageSaveProvider({
  saveImage,
  children,
}: {
  saveImage: ImageSave;
  children: ReactNode;
}) {
  return <ImageSaveContext value={saveImage}>{children}</ImageSaveContext>;
}
export function useImageSave(): ImageSave {
  const save = useContext(ImageSaveContext);
  if (!save) throw new Error("ImageSaveProvider is missing");
  return save;
}
