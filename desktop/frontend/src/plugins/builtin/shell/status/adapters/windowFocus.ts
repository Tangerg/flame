import { getContainer } from "@/main/container";

export function revealClientWindow(): Promise<void> {
  return getContainer().host.revealWindow();
}
