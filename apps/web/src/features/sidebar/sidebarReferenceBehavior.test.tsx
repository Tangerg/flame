import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SidebarActions } from "@/features/sidebar/actions";
import { ProjectsSection } from "@/features/sidebar/projects";
const model = vi.hoisted(() => ({
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
  index: {
    groups: [],
    recents: [],
    activeSessionId: "",
    activeCwd: undefined,
    isLoading: false,
  },
}));

vi.mock("@/features/navigation/public/workIndex", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/navigation/public/workIndex")>()),
  contributeWorkIndexItem: vi.fn(),
  useWorkIndex: () => model.index,
  useWorkIndexActions: () => model.actions,
}));

vi.mock("@/features/navigation/public/search/actions", () => ({
  SESSION_SEARCH_COMMAND: "chat.find",
  openSessionSearch: vi.fn(),
}));

vi.mock("@/features/workspace/public/navigation", () => ({
  openWorkspaceSettingsPane: vi.fn(),
}));

afterEach(() => {
  cleanup();
  model.actions.canCreateSession = true;
  model.actions.canCreateSessionInFolder = true;
});

describe("Codex-aligned Work Index actions", () => {
  it("keeps folder selection out of the global action stack", () => {
    testClient.render(<SidebarActions />);

    expect(screen.queryByRole("button", { name: "Open folder" })).toBeNull();
  });

  it("owns folder selection from the Projects section header", () => {
    testClient.render(<ProjectsSection />);

    expect(screen.getByRole("button", { name: "Add project" })).toBeTruthy();
  });

  it("keeps the Projects add action visible without hover discovery", () => {
    testClient.render(<ProjectsSection />);

    const addProject = screen.getByRole("button", { name: "Add project" });
    expect(addProject.classList.contains("opacity-0")).toBe(false);
  });

  it("withdraws Session creation affordances while Runtime commands are unavailable", () => {
    model.actions.canCreateSession = false;
    model.actions.canCreateSessionInFolder = false;
    testClient.render(
      <>
        <SidebarActions />
        <ProjectsSection />
      </>,
    );

    expect(
      (screen.getByRole("button", { name: "New session" }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "Add project" }) as HTMLButtonElement).disabled,
    ).toBe(true);
  });
});
