import {
  runtimeServiceStatus,
  type RuntimeServiceStatusPort,
} from "@/features/connection/application/ports/serviceStatus";
import { runtimeServiceAcceptsCommands } from "@/features/connection/application/runtimeCommandAvailability";
export type {
  RuntimeServicePhase,
  RuntimeServiceSnapshot,
} from "@/features/connection/application/ports/serviceStatus";

export const useRuntimeServiceStatus: RuntimeServiceStatusPort["useSnapshot"] = () =>
  runtimeServiceStatus().useSnapshot();

export function useRuntimeCommandsAvailable(): boolean {
  return runtimeServiceAcceptsCommands(useRuntimeServiceStatus());
}

export function runtimeCommandsAvailable(): boolean {
  return runtimeServiceAcceptsCommands(runtimeServiceStatus().snapshot());
}

export function refreshRuntimeServiceStatus(): Promise<void> {
  return runtimeServiceStatus().refresh();
}
