import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  ComposerProjectTray,
  EmptyChatHeading,
} from "@/features/shell/workbench/panel/ProjectSelector";
const mocks = vi.hoisted(() => ({
  workIndex: {
    groups: [
      { project: { id: "/repo/scope", name: "scope" }, sessions: [] },
      { project: { id: "/repo/other", name: "other" }, sessions: [] },
    ],
    recents: [],
    activeSessionId: "",
    activeCwd: undefined as string | undefined,
    isLoading: false,
  },
  actions: {
    canCreateSession: true,
    canCreateSessionInFolder: true,
    createSession: vi.fn(),
    chooseSessionFolder: vi.fn(),
    startSessionInFolder: vi.fn(),
    selectSession: vi.fn(),
    renameSession: vi.fn(),
    forkSession: vi.fn(),
    deleteSession: vi.fn(),
    toggleFavorite: vi.fn(),
    openSettings: vi.fn(),
  },
}));

vi.mock("@/features/navigation/public/workIndex", () => ({
  useWorkIndex: () => mocks.workIndex,
  useWorkIndexActions: () => mocks.actions,
}));

beforeEach(() => {
  mocks.workIndex.activeSessionId = "";
  mocks.workIndex.activeCwd = undefined;
  mocks.workIndex.isLoading = false;
  mocks.actions.canCreateSessionInFolder = true;
  mocks.actions.chooseSessionFolder.mockReset();
  mocks.actions.startSessionInFolder.mockReset();
});

describe("ComposerProjectTray", () => {
  it("owns a full-width attached tray instead of a composer-footer utility", () => {
    const { container } = testClient.render(<ComposerProjectTray />);

    const tray = container.querySelector<HTMLElement>('[data-slot="project-selector-tray"]');
    const surface = tray?.closest<HTMLElement>('[data-slot="composer-top-tray-surface"]');
    expect(tray).not.toBeNull();
    expect(surface).not.toBeNull();
    expect(surface?.dataset.tray).toBe("attached");
    expect(tray?.querySelectorAll("svg")).toHaveLength(1);
  });

  it("offers real Work Index projects and the native new-project action before a Session exists", async () => {
    testClient.render(<ComposerProjectTray />);

    fireEvent.click(screen.getByRole("button", { name: "Choose project" }));
    fireEvent.click(await screen.findByText("other"));

    expect(mocks.actions.startSessionInFolder).toHaveBeenCalledWith("/repo/other");
  });

  it("routes New project to the native folder owner", async () => {
    testClient.render(<ComposerProjectTray />);

    fireEvent.click(screen.getByRole("button", { name: "Choose project" }));
    fireEvent.click(await screen.findByText("New project"));

    expect(mocks.actions.chooseSessionFolder).toHaveBeenCalledOnce();
  });

  it("withdraws the utility-row picker once a Session owns the workspace", () => {
    mocks.workIndex.activeSessionId = "session-current";

    const { container } = testClient.render(<ComposerProjectTray />);

    expect(container.innerHTML).toBe("");
  });
});

describe("EmptyChatHeading", () => {
  it("renders the projectless Codex invitation without inventing a project", () => {
    testClient.render(<EmptyChatHeading />);

    expect(screen.getByText("What should we build?")).not.toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("makes the active Session project the exact-cwd picker", async () => {
    mocks.workIndex.activeSessionId = "session-current";
    mocks.workIndex.activeCwd = "/repo/scope";
    testClient.render(<EmptyChatHeading />);

    fireEvent.click(screen.getByRole("button", { name: "Change project: scope" }));
    fireEvent.click(await screen.findByText("other"));

    await waitFor(() =>
      expect(mocks.actions.startSessionInFolder).toHaveBeenCalledWith("/repo/other"),
    );
  });
});
