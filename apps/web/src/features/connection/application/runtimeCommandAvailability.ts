import { type RuntimeServiceSnapshot } from "@/features/connection/application/runtimeService";
export function runtimeServiceAcceptsCommands(snapshot: RuntimeServiceSnapshot): boolean {
  return snapshot.observation !== null && snapshot.phase !== "unavailable";
}
