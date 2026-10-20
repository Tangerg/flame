import { type RuntimeServiceSnapshot } from "@/features/connection/application/ports/serviceStatus";
export function runtimeServiceAcceptsCommands(snapshot: RuntimeServiceSnapshot): boolean {
  return snapshot.observation !== null && snapshot.phase !== "unavailable";
}
