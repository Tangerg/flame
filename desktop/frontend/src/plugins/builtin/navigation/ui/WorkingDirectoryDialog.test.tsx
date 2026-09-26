import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { workingDirectoryPicker } from "../application/ports/workingDirectoryPicker";
import {
  installWorkingDirectoryPicker,
  useWorkingDirectorySelection,
} from "../adapters/workingDirectorySelection";
import { WorkingDirectoryDialog } from "./WorkingDirectoryDialog";

const runtime = vi.hoisted(() => ({
  create: vi.fn(),
  browse: vi.fn(),
  local: false,
  endpoint: "https://runtime.example",
  focus: vi.fn(),
}));
vi.mock("@/main/container", () => ({
  getContainer: () => ({
    host: { chooseWorkingDirectory: runtime.browse },
    localWorkspaceAvailable: () => runtime.local,
  }),
}));
vi.mock("@/plugins/builtin/runtime/public/endpoint", () => ({
  currentRuntimeEndpoint: () => runtime.endpoint,
}));
vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => true,
}));
vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useCreateSession: () => runtime.create,
}));
vi.mock("@/plugins/builtin/chat/composer/public/focus", () => ({ focusComposer: runtime.focus }));

let picker: ReturnType<typeof installWorkingDirectoryPicker>;
beforeEach(() => {
  runtime.create.mockReset().mockResolvedValue("session-new");
  runtime.browse.mockReset();
  runtime.focus.mockReset();
  runtime.local = false;
  picker = installWorkingDirectoryPicker();
});
afterEach(() => picker.dispose());

function open() {
  render(<WorkingDirectoryDialog />);
  act(() => workingDirectoryPicker().open());
}

describe("Runtime workspace selection", () => {
  it("creates the first browser Session from an explicit Runtime directory", async () => {
    open();
    expect(screen.queryByRole("button", { name: "Browse this computer" })).toBeNull();
    expect(screen.getByText(/https:\/\/runtime.example/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Runtime workspace directory"), {
      target: { value: " /srv/project " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create session" }));
    await waitFor(() => expect(runtime.create).toHaveBeenCalledWith({ cwd: "/srv/project" }));
    await waitFor(() => expect(useWorkingDirectorySelection.getState().selection).toBeNull());
    expect(runtime.focus).toHaveBeenCalledOnce();
    expect(runtime.browse).not.toHaveBeenCalled();
  });

  it("preserves the selected path when Runtime rejects the directory", async () => {
    runtime.create.mockResolvedValue(null);
    open();
    fireEvent.change(screen.getByLabelText("Runtime workspace directory"), {
      target: { value: "/missing" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create session" }));
    await waitFor(() =>
      expect(useWorkingDirectorySelection.getState().selection?.busy).toBe(false),
    );
    expect((screen.getByLabelText("Runtime workspace directory") as HTMLInputElement).value).toBe(
      "/missing",
    );
    expect(runtime.focus).not.toHaveBeenCalled();
  });

  it("cancels without a Session mutation", () => {
    open();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(useWorkingDirectorySelection.getState().selection).toBeNull();
    expect(runtime.create).not.toHaveBeenCalled();
  });

  it("ignores native chooser completion after the Runtime target changes", async () => {
    runtime.local = true;
    const native = Promise.withResolvers<string>();
    runtime.browse.mockReturnValue(native.promise);
    open();
    fireEvent.click(screen.getByRole("button", { name: "Browse this computer" }));
    act(() => {
      picker.cancel();
      runtime.local = false;
      workingDirectoryPicker().open();
    });
    await act(async () => native.resolve("/old/local/path"));
    expect(useWorkingDirectorySelection.getState().selection?.path).toBe("");
    expect(useWorkingDirectorySelection.getState().selection?.canBrowse).toBe(false);
    expect(runtime.create).not.toHaveBeenCalled();
  });

  it("keeps successor dialog ownership when an older plugin is disposed", () => {
    open();
    const successor = installWorkingDirectoryPicker();
    act(() => {
      workingDirectoryPicker().open();
      picker.dispose();
    });
    expect(useWorkingDirectorySelection.getState().selection).not.toBeNull();
    successor.dispose();
    expect(useWorkingDirectorySelection.getState().selection).toBeNull();
  });
});
