import { onlineManager } from "@tanstack/react-query";
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { AGENT_SESSIONS_KEY, type AgentSessionSummary } from "@/features/agent/public/session";
import { SessionSearch } from "@/features/navigation/session-search/SessionSearch";
import { useSessionSearchStore } from "@/features/navigation/session-search/sessionSearchState";
import { openSessionSearch } from "@/features/navigation/public/search/actions";
import { queryClient } from "@/lib/queryClient";
import { contributeForTest } from "@/test/kernel";
import { createTestClient, type TestClient } from "@/test/renderClient";

let client: TestClient;
const readSessions = vi.fn<() => Promise<AgentSessionSummary[]>>();
const sessionsKey = [AGENT_SESSIONS_KEY];
const session: AgentSessionSummary = {
  id: "available-session",
  revision: 1,
  title: "Available conversation",
  status: "idle",
  provider: "test-provider",
  model: "test-model",
  workspace: { path: "/repo/flame", availability: "available" },
  time: "2026-10-28T00:00:00.000Z",
};

beforeEach(async (context) => {
  const previousOnline = onlineManager.isOnline();
  const previousDefaults = queryClient.getQueryDefaults(sessionsKey);
  onlineManager.setOnline(false);
  queryClient.setQueryDefaults(sessionsKey, { ...previousDefaults, retry: false });
  readSessions.mockReset().mockRejectedValue(new Error("Runtime unavailable"));
  useSessionSearchStore.getState().setOpen(false);
  await contributeForTest((ctx) => {
    ctx.contribute(DATA_PROVIDER, { key: AGENT_SESSIONS_KEY, fetcher: readSessions });
  });
  const owned = createTestClient({ start: false });
  client = owned;
  context.onTestFinished(async () => {
    useSessionSearchStore.getState().setOpen(false);
    await owned.dispose();
    onlineManager.setOnline(previousOnline);
    queryClient.setQueryDefaults(sessionsKey, previousDefaults);
  });
});

async function openSearch() {
  client.render(
    <>
      <button onClick={openSessionSearch}>Search sessions</button>
      <SessionSearch />
    </>,
  );
  const opener = screen.getByRole("button", { name: "Search sessions" });
  act(() => opener.focus());
  fireEvent.click(opener);
  const field = (await screen.findByRole("combobox")) as HTMLInputElement;
  return { opener, field, dialog: screen.getByRole("dialog") };
}

async function failFirstRead() {
  const search = await openSearch();
  act(() => onlineManager.setOnline(true));
  await screen.findByRole("button", { name: "Retry" });
  return search;
}

it("keeps an unknown paused snapshot loading and retains the typed query when results arrive", async () => {
  const { field } = await openSearch();
  expect(queryClient.getQueryState(sessionsKey)).toMatchObject({
    status: "pending",
    fetchStatus: "paused",
    data: undefined,
  });
  expect(screen.getByRole("status").getAttribute("aria-busy")).toBe("true");
  expect(screen.queryByText("No session matches")).toBeNull();
  expect(field.getAttribute("aria-expanded")).toBe("false");
  expect(field.getAttribute("aria-activedescendant")).toBeNull();
  fireEvent.change(field, { target: { value: "conversation" } });
  act(() => field.setSelectionRange(1, 5));
  act(() => queryClient.setQueryData(sessionsKey, [session]));

  const result = await screen.findByRole("option", { name: /Available conversation/ });
  expect(screen.getByRole("combobox")).toBe(field);
  expect(field.value).toBe("conversation");
  expect([field.selectionStart, field.selectionEnd]).toEqual([1, 5]);
  expect(document.activeElement).toBe(field);
  expect(field.getAttribute("aria-expanded")).toBe("true");
  expect(field.getAttribute("aria-activedescendant")).toBe(result.id);
  expect(screen.queryByRole("status")).toBeNull();
  expect(readSessions).not.toHaveBeenCalled();
});

it("shows no matches for an accepted empty snapshot", async () => {
  queryClient.setQueryData(sessionsKey, []);
  const { field } = await openSearch();
  expect(screen.getByText("No session matches")).toBeTruthy();
  expect(screen.queryByRole("status")).toBeNull();
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  expect(field.getAttribute("aria-activedescendant")).toBeNull();
});

it.each(["empty", "results", "failure"] as const)(
  "keeps the query and dialog focus through retry and %s settlement",
  async (outcome) => {
    const { field, dialog } = await failFirstRead();
    const response = Promise.withResolvers<AgentSessionSummary[]>();
    readSessions.mockReturnValueOnce(response.promise);
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.queryByText("No session matches")).toBeNull();
    const retry = screen.getByRole("button", { name: "Retry" });
    expect(retry.closest('[role="listbox"]')).toBeNull();
    fireEvent.change(field, { target: { value: "conversation" } });
    act(() => retry.focus());
    fireEvent.click(retry);
    await screen.findByRole("status");
    expect(document.activeElement).toBe(dialog);
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.getByRole("combobox")).toBe(field);
    expect(field.value).toBe("conversation");

    act(() => {
      if (outcome === "failure") response.reject(new Error("Runtime unavailable"));
      else response.resolve(outcome === "empty" ? [] : [session]);
    });
    if (outcome === "failure") await screen.findByRole("button", { name: "Retry" });
    else if (outcome === "empty") await screen.findByText("No session matches");
    else await screen.findByRole("option", { name: /Available conversation/ });
    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.getByRole("combobox")).toBe(field);
    expect(field.value).toBe("conversation");
    expect(document.activeElement).toBe(dialog);
    expect(readSessions).toHaveBeenCalledTimes(2);
  },
);

it("retains later input focus and selection when a pending retry fails again", async () => {
  const { field } = await failFirstRead();
  const response = Promise.withResolvers<AgentSessionSummary[]>();
  readSessions.mockReturnValueOnce(response.promise);
  const retry = screen.getByRole("button", { name: "Retry" });
  act(() => retry.focus());
  fireEvent.click(retry);
  await screen.findByRole("status");
  fireEvent.change(field, { target: { value: "retained query" } });
  act(() => {
    field.focus();
    field.setSelectionRange(2, 7);
    response.reject(new Error("Runtime unavailable"));
  });
  await screen.findByRole("button", { name: "Retry" });
  expect(document.activeElement).toBe(field);
  expect(field.value).toBe("retained query");
  expect([field.selectionStart, field.selectionEnd]).toEqual([2, 7]);
});

it("retains accepted results and input focus through a failed background read", async () => {
  queryClient.setQueryData(sessionsKey, [session]);
  const { field } = await openSearch();
  fireEvent.change(field, { target: { value: "conversation" } });
  const result = screen.getByRole("option", { name: /Available conversation/ });
  act(() => {
    void queryClient.invalidateQueries({ queryKey: sessionsKey });
    onlineManager.setOnline(true);
  });
  await waitFor(() => expect(queryClient.getQueryState(sessionsKey)?.status).toBe("error"));
  expect(screen.getByRole("option", { name: /Available conversation/ })).toBe(result);
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.queryByRole("status")).toBeNull();
  expect(document.activeElement).toBe(field);
  expect(field.value).toBe("conversation");
});

it("returns to the opener on Escape during retry and ignores late results until reopening", async () => {
  const { opener, field } = await failFirstRead();
  const response = Promise.withResolvers<AgentSessionSummary[]>();
  readSessions.mockReturnValueOnce(response.promise);
  fireEvent.change(field, { target: { value: "conversation" } });
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  await screen.findByRole("status");
  fireEvent.keyDown(field, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(document.activeElement).toBe(opener);
  act(() => response.resolve([session]));
  await waitFor(() => expect(queryClient.getQueryData(sessionsKey)).toEqual([session]));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(opener);

  fireEvent.click(opener);
  const reopened = (await screen.findByRole("combobox")) as HTMLInputElement;
  expect(reopened.value).toBe("");
  expect(screen.getByRole("option", { name: /Available conversation/ })).toBeTruthy();
  expect(document.activeElement).toBe(reopened);
});
