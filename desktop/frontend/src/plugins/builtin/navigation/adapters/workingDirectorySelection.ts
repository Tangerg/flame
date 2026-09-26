import { create } from "zustand";
import { getContainer } from "@/main/container";
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

export async function browseWorkingDirectory(expected: DirectorySelection): Promise<void> {
  const current = useWorkingDirectorySelection.getState().selection;
  if (
    current?.owner !== expected.owner ||
    !expected.canBrowse ||
    !getContainer().localWorkspaceAvailable()
  )
    return;
  const path = await getContainer().host.chooseWorkingDirectory();
  if (path) updateWorkingDirectorySelection(expected, { path });
}

export function installWorkingDirectoryPicker(): { dispose(): void; cancel(): void } {
  let currentOwner: object | undefined;
  const cancel = () => {
    const selection = useWorkingDirectorySelection.getState().selection;
    if (selection?.owner === currentOwner)
      useWorkingDirectorySelection.setState({ selection: null });
    currentOwner = undefined;
  };
  const disconnect = configureWorkingDirectoryPicker({
    open() {
      const current = useWorkingDirectorySelection.getState().selection;
      if (current && current.owner === currentOwner) return;
      currentOwner = {};
      useWorkingDirectorySelection.setState({
        selection: {
          owner: currentOwner,
          endpoint: currentRuntimeEndpoint(),
          canBrowse: getContainer().localWorkspaceAvailable(),
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
