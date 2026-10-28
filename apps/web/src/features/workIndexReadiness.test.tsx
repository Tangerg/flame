import { onlineManager } from "@tanstack/react-query";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { AGENT_SESSIONS_KEY } from "@/features/agent/public/session";
import { useWorkIndex } from "@/features/navigation/public/workIndex";
import { ProjectsSection } from "@/features/sidebar/projects";
import { ComposerProjectTray } from "@/features/shell/workbench/panel/ProjectSelector";
import {
  WORKSPACE_PROJECTS_KEY,
  type WorkspaceProjectSummary,
} from "@/features/workspace/public/queries";
import { queryClient } from "@/lib/queryClient";
import { contributeForTest } from "@/test/kernel";
import { createTestClient, type TestClient } from "@/test/renderClient";

const actions = vi.hoisted(() => ({
  canCreateSessionInFolder: true,
  chooseSessionFolder: vi.fn(),
  startSessionInFolder: vi.fn(),
}));

vi.mock("@/features/navigation/public/workIndex", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/navigation/public/workIndex")>()),
  useWorkIndexActions: () => actions,
}));

let client: TestClient;
const readProjects = vi.fn<() => Promise<WorkspaceProjectSummary[]>>();
const projectsKey = [WORKSPACE_PROJECTS_KEY];
const project: WorkspaceProjectSummary = { id: "/repo/flame", name: "Flame", sessionCount: 0 };
const retainedSession = {
  id: "available-session",
  revision: 1,
  title: "Available conversation",
  status: "idle" as const,
  provider: "test-provider",
  model: "test-model",
  workspace: { path: "/repo/flame", availability: "available" as const },
  time: "2026-10-28T00:00:00.000Z",
};

beforeEach(async (context) => {
  const previousOnline = onlineManager.isOnline();
  const previousDefaults = queryClient.getQueryDefaults(projectsKey);
  onlineManager.setOnline(false);
  queryClient.setQueryDefaults(projectsKey, { ...previousDefaults, retry: false });
  readProjects.mockReset().mockRejectedValue(new Error("Runtime unavailable"));
  actions.chooseSessionFolder.mockClear();
  actions.startSessionInFolder.mockClear();
  await contributeForTest((ctx) => {
    ctx.contribute(DATA_PROVIDER, { key: WORKSPACE_PROJECTS_KEY, fetcher: readProjects });
  });
  const owned = createTestClient({ start: false });
  client = owned;
  queryClient.setQueryData([AGENT_SESSIONS_KEY], []);
  context.onTestFinished(async () => {
    await owned.dispose();
    onlineManager.setOnline(previousOnline);
    queryClient.setQueryDefaults(projectsKey, previousDefaults);
  });
});

it("keeps a paused first read pending without claiming an empty work index", () => {
  const { result } = client.renderHook(() => useWorkIndex());
  expect(queryClient.getQueryState(projectsKey)).toMatchObject({
    status: "pending",
    fetchStatus: "paused",
    data: undefined,
  });
  expect(result.current.groups).toBeUndefined();
  expect(result.current.isLoading).toBe(true);
  expect(result.current.error).toBeUndefined();
  expect(readProjects).not.toHaveBeenCalled();
});

it("shows the sidebar loading state until a known empty project snapshot arrives", async () => {
  client.render(
    <>
      <button>Continue reading</button>
      <ProjectsSection />
    </>,
  );
  expect(screen.getByRole("status").getAttribute("aria-busy")).toBe("true");
  expect(screen.queryByText("No projects")).toBeNull();
  const outside = screen.getByRole("button", { name: "Continue reading" });
  act(() => outside.focus());
  act(() => queryClient.setQueryData(projectsKey, []));
  await screen.findByText("No projects");
  expect(screen.queryByRole("status")).toBeNull();
  expect(document.activeElement).toBe(outside);
});

it("keeps the project menu's loading row until actual folder choices arrive", async () => {
  queryClient.setQueryData([AGENT_SESSIONS_KEY], [retainedSession]);
  client.render(<ComposerProjectTray />);
  const trigger = screen.getByRole("button", { name: "Choose project" });
  fireEvent.click(trigger);
  const menu = await screen.findByRole("menu");
  expect(
    within(menu).getByRole("menuitem", { name: "Loading" }).getAttribute("aria-disabled"),
  ).toBe("true");
  act(() => queryClient.setQueryData(projectsKey, [project]));
  fireEvent.click(await within(menu).findByRole("menuitem", { name: "Flame" }));
  expect(actions.startSessionInFolder).toHaveBeenCalledExactlyOnceWith("/repo/flame");
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  expect(document.activeElement).toBe(trigger);
});

it("keeps available recents while projects still have no snapshot", () => {
  queryClient.setQueryData([AGENT_SESSIONS_KEY], [retainedSession]);
  const { result } = client.renderHook(() => useWorkIndex());
  client.render(<ProjectsSection />);
  expect(result.current.recents?.map((session) => session.id)).toEqual(["available-session"]);
  expect(result.current.groups).toBeUndefined();
  expect(screen.getByRole("status").getAttribute("aria-busy")).toBe("true");
  expect(screen.queryByText("No projects")).toBeNull();
});

it("keeps available recents while exposing a failed first project read", async () => {
  queryClient.setQueryData([AGENT_SESSIONS_KEY], [retainedSession]);
  const { result } = client.renderHook(() => useWorkIndex());
  client.render(<ProjectsSection />);
  act(() => onlineManager.setOnline(true));
  await screen.findByText("Couldn't load projects");
  expect(result.current.recents?.map((session) => session.id)).toEqual(["available-session"]);
  expect(result.current.groups).toBeUndefined();
  expect(result.current.error).toBeInstanceOf(Error);
  expect(screen.queryByText("No projects")).toBeNull();
});

it("retains accepted project rows and later focus through a failed background read", async () => {
  queryClient.setQueryData(projectsKey, [project]);
  client.render(
    <>
      <button>Continue reading</button>
      <ProjectsSection />
    </>,
  );
  const row = screen.getByText("Flame");
  const outside = screen.getByRole("button", { name: "Continue reading" });
  act(() => outside.focus());
  act(() => {
    void queryClient.invalidateQueries({ queryKey: projectsKey });
    onlineManager.setOnline(true);
  });
  await waitFor(() => expect(queryClient.getQueryState(projectsKey)?.status).toBe("error"));
  expect(screen.getByText("Flame")).toBe(row);
  expect(screen.queryByRole("status")).toBeNull();
  expect(screen.queryByText("Couldn't load projects")).toBeNull();
  expect(document.activeElement).toBe(outside);
});

it("replaces pending with a real failure and lets the existing Retry accept an empty snapshot", async () => {
  client.render(<ProjectsSection />);
  act(() => onlineManager.setOnline(true));
  await screen.findByText("Couldn't load projects");
  expect(screen.queryByRole("status")).toBeNull();
  expect(screen.queryByText("No projects")).toBeNull();
  readProjects.mockResolvedValueOnce([]);
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  await screen.findByText("No projects");
  expect(screen.queryByText("Couldn't load projects")).toBeNull();
  expect(readProjects).toHaveBeenCalledTimes(2);
});
