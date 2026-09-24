import { getContainer } from "@/main/container";

export function revealDesktopWindow(): Promise<void> {
  return getContainer().desktop.revealWindow();
}
