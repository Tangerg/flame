import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
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

const defaultGroups = mocks.workIndex.groups;
const sameNameGroups = [
  { project: { id: "/repo/main/flame", name: "flame" }, sessions: [] },
  { project: { id: "/repo/worktrees/visual/flame", name: "flame" }, sessions: [] },
  defaultGroups[1]!,
];

beforeEach(() => {
  mocks.workIndex.groups = defaultGroups;
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

  it("distinguishes same-name projects by visible paths and selects their exact folder", async () => {
    mocks.workIndex.groups = sameNameGroups;
    testClient.render(<ComposerProjectTray />);
    fireEvent.click(screen.getByRole("button", { name: "Choose project" }));

    const menu = await screen.findByRole("menu");
    const primary = within(menu).getByRole("menuitem", { name: /\/repo\/main\/flame/ });
    const worktree = within(menu).getByRole("menuitem", {
      name: /\/repo\/worktrees\/visual\/flame/,
    });
    expect(within(primary).getByText("/repo/main/flame")).toBeTruthy();
    expect(within(worktree).getByText("/repo/worktrees/visual/flame")).toBeTruthy();
    expect(within(menu).getByRole("menuitem", { name: "other" })).toBeTruthy();
    expect(within(menu).queryByText("/repo/other")).toBeNull();
    fireEvent.click(worktree);

    expect(mocks.actions.startSessionInFolder).toHaveBeenCalledWith("/repo/worktrees/visual/flame");
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

  it("identifies the current folder among same-name projects without creating another Session", async () => {
    mocks.workIndex.groups = sameNameGroups;
    mocks.workIndex.activeSessionId = "session-current";
    mocks.workIndex.activeCwd = "/repo/main/flame";
    testClient.render(<EmptyChatHeading />);
    const trigger = screen.getByRole("button", { name: "Change project: flame" });
    fireEvent.click(trigger);

    const current = await screen.findByRole("menuitem", { name: /\/repo\/main\/flame/ });
    const other = screen.getByRole("menuitem", { name: /\/repo\/worktrees\/visual\/flame/ });
    expect(current.getAttribute("aria-current")).toBe("true");
    expect(other.getAttribute("aria-current")).toBeNull();
    fireEvent.click(current);
    expect(mocks.actions.startSessionInFolder).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    fireEvent.click(trigger);
    fireEvent.click(
      await screen.findByRole("menuitem", { name: /\/repo\/worktrees\/visual\/flame/ }),
    );
    expect(mocks.actions.startSessionInFolder).toHaveBeenCalledWith("/repo/worktrees/visual/flame");
  });

  it("keeps named folder choices inert when creating a Session is unavailable", async () => {
    mocks.workIndex.groups = sameNameGroups;
    mocks.workIndex.activeSessionId = "session-current";
    mocks.workIndex.activeCwd = "/repo/main/flame";
    mocks.actions.canCreateSessionInFolder = false;
    testClient.render(<EmptyChatHeading />);
    fireEvent.click(screen.getByRole("button", { name: "Change project: flame" }));

    const worktree = await screen.findByRole("menuitem", {
      name: /\/repo\/worktrees\/visual\/flame/,
    });
    expect(worktree.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(worktree);
    expect(mocks.actions.startSessionInFolder).not.toHaveBeenCalled();
  });
});
