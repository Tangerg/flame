import { getContainer } from "@/main/container";

export function saveInlineImage(source: string): Promise<boolean> {
  return getContainer().desktop.saveImage(source);
}
