import { createSingletonPort } from "@/lib/ports/singletonPort";

export type ImageSave = (source: string) => Promise<boolean>;

const port = createSingletonPort<ImageSave>("Image saving is not configured");

export const configureImageSave = port.configure;

export function saveInlineImage(source: string): Promise<boolean> {
  return port.get()(source);
}
