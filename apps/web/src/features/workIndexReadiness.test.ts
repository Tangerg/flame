import { onlineManager } from "@tanstack/react-query";

import { beforeEach, expect, it, vi } from "vitest";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { AGENT_SESSIONS_KEY } from "@/features/agent/public/session";
import { useWorkIndex } from "@/features/navigation/public/workIndex";

import {
  WORKSPACE_PROJECTS_KEY,
  type WorkspaceProjectSummary,
} from "@/features/workspace/public/queries";
import { queryClient } from "@/lib/queryClient";
import { contributeForTest } from "@/test/kernel";
import { createTestClient, type TestClient } from "@/test/renderClient";

let client: TestClient;
const readProjects = vi.fn<() => Promise<WorkspaceProjectSummary[]>>();
const projectsKey = [WORKSPACE_PROJECTS_KEY];

beforeEach(async (context) => {
  const previousOnline = onlineManager.isOnline();
  const previousDefaults = queryClient.getQueryDefaults(projectsKey);
  onlineManager.setOnline(false);
  queryClient.setQueryDefaults(projectsKey, { ...previousDefaults, retry: false });
  readProjects.mockReset().mockRejectedValue(new Error("Runtime unavailable"));
  await contributeForTest((ctx) => {
    ctx.contributions.add(DATA_PROVIDER, { key: WORKSPACE_PROJECTS_KEY, fetcher: readProjects });
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
