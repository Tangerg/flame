import { create } from "zustand";
import { type WorkingDirectoryAccess } from "@/features/navigation/application/workingDirectoryPicker";
import { type DirectorySelection } from "@/features/navigation/application/workingDirectoryPicker";
export function createWorkingDirectoryPicker(
  chooseDirectory: () => Promise<string | null>,
  canBrowse: () => boolean,
  currentRuntimeEndpoint: () => string,
) {
  const useWorkingDirectorySelection = create<{ selection: DirectorySelection | null }>(() => ({
    selection: null,
  }));

  function updateWorkingDirectorySelection(
    expected: DirectorySelection,
    update: { path?: string; busy?: boolean; error?: string | null } | null,
  ): boolean {
    const current = useWorkingDirectorySelection.getState().selection;
    if (current?.owner !== expected.owner) return false;
    useWorkingDirectorySelection.setState({ selection: update ? { ...current, ...update } : null });
    return true;
  }

  let currentOwner: object | undefined;
  const cancel = () => {
    const selection = useWorkingDirectorySelection.getState().selection;
    if (selection?.owner === currentOwner)
      useWorkingDirectorySelection.setState({ selection: null });
    currentOwner = undefined;
  };
  const access: WorkingDirectoryAccess = {
    useSelection: () => useWorkingDirectorySelection((state) => state.selection),
    selection: () => useWorkingDirectorySelection.getState().selection,
    update: updateWorkingDirectorySelection,
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
          error: null,
        },
      });
    },
  };
  return {
    access,
    store: useWorkingDirectorySelection,
    cancel,
    dispose() {
      cancel();
    },
  };
}
