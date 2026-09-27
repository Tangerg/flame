import { create } from "zustand";
import { currentRuntimeEndpoint } from "@/plugins/builtin/runtime/public/endpoint";
import { configureWorkingDirectoryPicker } from "../application/ports/workingDirectoryPicker";

interface DirectorySelection {
  readonly owner: object;
  readonly endpoint: string;
  readonly canBrowse: boolean;
  readonly path: string;
  readonly busy: boolean;
}

export const useWorkingDirectorySelection = create<{ selection: DirectorySelection | null }>(
  () => ({
    selection: null,
  }),
);

export function updateWorkingDirectorySelection(
  expected: DirectorySelection,
  update: { path?: string; busy?: boolean } | null,
): boolean {
  const current = useWorkingDirectorySelection.getState().selection;
  if (current?.owner !== expected.owner) return false;
  useWorkingDirectorySelection.setState({ selection: update ? { ...current, ...update } : null });
  return true;
}

export function installWorkingDirectoryPicker(
  chooseDirectory: () => Promise<string | null>,
  canBrowse: () => boolean,
): { dispose(): void; cancel(): void } {
  let currentOwner: object | undefined;
  const cancel = () => {
    const selection = useWorkingDirectorySelection.getState().selection;
    if (selection?.owner === currentOwner)
      useWorkingDirectorySelection.setState({ selection: null });
    currentOwner = undefined;
  };
  const disconnect = configureWorkingDirectoryPicker({
    async browse(expectedOwner) {
      const current = useWorkingDirectorySelection.getState().selection;
      if (
        !current ||
        current.owner !== expectedOwner ||
        current.owner !== currentOwner ||
        !current.canBrowse ||
        !canBrowse()
      )
        return;
      const path = await chooseDirectory();
      if (path) updateWorkingDirectorySelection(current, { path });
    },
    open() {
      const current = useWorkingDirectorySelection.getState().selection;
      if (current && current.owner === currentOwner) return;
      currentOwner = {};
      useWorkingDirectorySelection.setState({
        selection: {
          owner: currentOwner,
          endpoint: currentRuntimeEndpoint(),
          canBrowse: canBrowse(),
          path: "",
          busy: false,
        },
      });
    },
  });
  return {
    cancel,
    dispose() {
      cancel();
      disconnect();
    },
  };
}
