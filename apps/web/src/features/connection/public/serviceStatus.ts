import { type RuntimeAccess, useRuntime } from "@/features/connection/public/runtime";
import { runtimeServiceAcceptsCommands } from "@/features/connection/application/runtimeCommandAvailability";
export type {
  RuntimeServicePhase,
  RuntimeServiceSnapshot,
} from "@/features/connection/application/runtimeService";
export function useRuntimeServiceStatus() {
  return useRuntime().status.useSnapshot();
}
export function useRuntimeCommandsAvailable(): boolean {
  return runtimeServiceAcceptsCommands(useRuntimeServiceStatus());
}
export function runtimeCommandsAvailable(runtime: RuntimeAccess): boolean {
  return runtimeServiceAcceptsCommands(runtime.status.snapshot());
}
