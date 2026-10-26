import { WorkingDirectoryProvider } from "@/features/navigation/public/workingDirectory";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createWorkingDirectoryPicker } from "@/features/navigation/adapters/workingDirectorySelection";
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

let picker: ReturnType<typeof createWorkingDirectoryPicker>;

beforeEach(() => {
  runtime.create.mockReset().mockResolvedValue("session-new");
  runtime.browse.mockReset();
  runtime.focus.mockReset();
  runtime.local = false;
  runtime.endpoint = "https://runtime.example";
  picker = createWorkingDirectoryPicker(
    runtime.browse,
    () => runtime.local,
    () => runtime.endpoint,
  );
});

afterEach(() => picker.dispose());

function open() {
  testClient.render(
    <WorkingDirectoryProvider value={picker.access}>
      <WorkingDirectoryDialog />
    </WorkingDirectoryProvider>,
  );
  act(() => picker.access.open());
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
    await waitFor(() => expect(picker.store.getState().selection).toBeNull());
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
    await waitFor(() => expect(picker.store.getState().selection?.busy).toBe(false));
    expect(screen.getByRole("alert").textContent).toContain(t("session.error.create"));
    expect((screen.getByLabelText("Runtime workspace directory") as HTMLInputElement).value).toBe(
      "/missing",
    );
    expect(runtime.focus).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Create session" }));
    await waitFor(() => expect(runtime.create).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(picker.store.getState().selection?.busy).toBe(false));
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
    expect(picker.store.getState().selection?.error).toBeNull();
  });

  it("reports chooser failure inside the dialog and clears it on retry", async () => {
    runtime.local = true;
    runtime.endpoint = `https://runtime.example/${"organization/workspace/deployment-region/".repeat(25)}runtime-scope-end`;
    const task = `${t("session.directory.title")} ${runtime.endpoint}`;
    runtime.browse
      .mockRejectedValueOnce(new Error("permission denied"))
      .mockResolvedValueOnce("/picked");
    open();
    expect(screen.getByRole("dialog", { name: task })).toBeTruthy();
    const endpoint = screen.getByRole("region", { name: runtime.endpoint });
    expect(endpoint.textContent).toBe(runtime.endpoint);
    expect(endpoint.tabIndex).toBe(0);
    fireEvent.change(screen.getByLabelText("Runtime workspace directory"), {
      target: { value: "/srv/chosen-project" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Browse this computer" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain(t("session.error.chooseWorkingDirectory"));
    expect(alert.textContent).toContain("permission denied");
    expect(screen.getByRole("dialog", { name: task })).toBeTruthy();
    const field = screen.getByLabelText("Runtime workspace directory");
    expect((field as HTMLInputElement).value).toBe("/srv/chosen-project");
    expect(field.getAttribute("aria-invalid")).toBeNull();
    expect(field.getAttribute("aria-describedby")).toBe(alert.id);

    await waitFor(() => expect(picker.store.getState().selection?.busy).toBe(false));
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
    act(() => picker.access.open());
    await act(async () => native.reject(new Error("stale chooser")));
    expect(picker.store.getState().selection).toMatchObject({
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
      act(() => picker.access.open());
      await act(async () => pending.resolve(outcome));
      expect(picker.store.getState().selection).toMatchObject({
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
    expect(picker.store.getState().selection).toBeNull();
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
      runtime.endpoint = "https://successor-runtime.example";
      picker.access.open();
    });
    const task = `${t("session.directory.title")} https://successor-runtime.example`;
    expect(screen.getByRole("dialog", { name: task })).toBeTruthy();
    await act(async () => native.resolve("/old/local/path"));
    expect(screen.getByRole("dialog", { name: task })).toBeTruthy();
    expect(screen.queryByText("https://runtime.example")).toBeNull();
    expect((screen.getByLabelText("Runtime workspace directory") as HTMLInputElement).value).toBe(
      "",
    );
    expect(picker.store.getState().selection?.path).toBe("");
    expect(picker.store.getState().selection?.canBrowse).toBe(false);
    expect(runtime.create).not.toHaveBeenCalled();
  });

  it("keeps a successor instance's dialog when an older picker is disposed", () => {
    open();
    const successor = createWorkingDirectoryPicker(
      runtime.browse,
      () => runtime.local,
      () => runtime.endpoint,
    );
    act(() => {
      successor.access.open();
      picker.dispose();
    });
    expect(successor.store.getState().selection).not.toBeNull();
    successor.dispose();
    expect(successor.store.getState().selection).toBeNull();
  });
});
