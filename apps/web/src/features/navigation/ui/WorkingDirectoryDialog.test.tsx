import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { workingDirectoryPicker } from "@/features/navigation/application/ports/workingDirectoryPicker";
import {
  installWorkingDirectoryPicker,
  useWorkingDirectorySelection,
} from "@/features/navigation/adapters/workingDirectorySelection";
import { WorkingDirectoryDialog } from "@/features/navigation/ui/WorkingDirectoryDialog";
import { t } from "@/lib/i18n/index";
const runtime = vi.hoisted(() => ({
  create: vi.fn(),
  browse: vi.fn(),
  local: false,
  endpoint: "https://runtime.example",
  focus: vi.fn(),
}));

vi.mock("@/features/connection/public/endpoint", () => ({
  currentRuntimeEndpoint: () => runtime.endpoint,
}));

vi.mock("@/features/connection/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => true,
}));

vi.mock("@/features/agent/public/session", () => ({
  useCreateSession: () => runtime.create,
}));

vi.mock("@/features/composer/public/focus", () => ({ focusComposer: runtime.focus }));

let picker: ReturnType<typeof installWorkingDirectoryPicker>;

beforeEach(() => {
  runtime.create.mockReset().mockResolvedValue("session-new");
  runtime.browse.mockReset();
  runtime.focus.mockReset();
  runtime.local = false;
  picker = installWorkingDirectoryPicker(runtime.browse, () => runtime.local);
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
    expect(screen.getByRole("alert").textContent).toContain(t("session.error.create"));
    expect((screen.getByLabelText("Runtime workspace directory") as HTMLInputElement).value).toBe(
      "/missing",
    );
    expect(runtime.focus).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Create session" }));
    await waitFor(() => expect(runtime.create).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(useWorkingDirectorySelection.getState().selection?.busy).toBe(false),
    );
    expect(runtime.create).toHaveBeenLastCalledWith({ cwd: "/missing" });
  });

  it("clears creation feedback when the path is edited", async () => {
    runtime.create.mockResolvedValue(null);
    open();
    const field = screen.getByLabelText("Runtime workspace directory");
    fireEvent.change(field, { target: { value: "/missing" } });
    fireEvent.click(screen.getByRole("button", { name: "Create session" }));
    await screen.findByRole("alert");
    fireEvent.change(field, { target: { value: "/srv/project" } });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(useWorkingDirectorySelection.getState().selection?.error).toBeNull();
  });

  it("reports chooser failure inside the dialog and clears it on retry", async () => {
    runtime.local = true;
    runtime.browse
      .mockRejectedValueOnce(new Error("permission denied"))
      .mockResolvedValueOnce("/picked");
    open();
    fireEvent.click(screen.getByRole("button", { name: "Browse this computer" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain(t("session.error.chooseWorkingDirectory"));
    expect(alert.textContent).toContain("permission denied");
    const field = screen.getByLabelText("Runtime workspace directory");
    expect(field.getAttribute("aria-invalid")).toBeNull();
    expect(field.getAttribute("aria-describedby")).toBe(alert.id);

    await waitFor(() =>
      expect(useWorkingDirectorySelection.getState().selection?.busy).toBe(false),
    );
    fireEvent.click(screen.getByRole("button", { name: "Browse this computer" }));
    await waitFor(() => expect((field as HTMLInputElement).value).toBe("/picked"));
    expect(screen.queryByRole("alert")).toBeNull();
    expect(runtime.create).not.toHaveBeenCalled();
  });

  it("keeps an old chooser failure out of the successor dialog", async () => {
    runtime.local = true;
    const native = Promise.withResolvers<string>();
    runtime.browse.mockReturnValue(native.promise);
    open();
    fireEvent.click(screen.getByRole("button", { name: "Browse this computer" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    act(() => workingDirectoryPicker().open());
    await act(async () => native.reject(new Error("stale chooser")));
    expect(useWorkingDirectorySelection.getState().selection).toMatchObject({
      busy: false,
      error: null,
    });
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it.each([null, "session-old"])(
    "keeps an old creation outcome (%s) out of the successor dialog",
    async (outcome) => {
      const pending = Promise.withResolvers<string | null>();
      runtime.create.mockReturnValue(pending.promise);
      open();
      fireEvent.change(screen.getByLabelText("Runtime workspace directory"), {
        target: { value: "/old" },
      });
      fireEvent.click(screen.getByRole("button", { name: "Create session" }));
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      act(() => workingDirectoryPicker().open());
      await act(async () => pending.resolve(outcome));
      expect(useWorkingDirectorySelection.getState().selection).toMatchObject({
        path: "",
        busy: false,
        error: null,
      });
      expect(screen.queryByRole("alert")).toBeNull();
      expect(runtime.focus).not.toHaveBeenCalled();
    },
  );

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
    const successor = installWorkingDirectoryPicker(runtime.browse, () => runtime.local);
    act(() => {
      workingDirectoryPicker().open();
      picker.dispose();
    });
    expect(useWorkingDirectorySelection.getState().selection).not.toBeNull();
    successor.dispose();
    expect(useWorkingDirectorySelection.getState().selection).toBeNull();
  });
});
