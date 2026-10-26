import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@flame/ui";
import { startTask, useTasksStore } from "@/extensions/tasksStore";
import { TasksPill } from "./TasksPill";

beforeEach(() => {
  vi.useFakeTimers();
  useTasksStore.setState({ tasks: new Map() });
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

const renderTasks = () =>
  testClient.render(
    <TooltipProvider>
      <TasksPill />
      <textarea aria-label="Draft" defaultValue="Keep this draft." />
    </TooltipProvider>,
  );
async function settle() {
  await act(() => vi.advanceTimersByTimeAsync(30));
}
async function openTasks(name: string) {
  fireEvent.click(screen.getByRole("button", { name }));
  await settle();
}

it("does not render an empty progress entry", () => {
  renderTasks();
  expect(screen.queryByRole("button")).toBeNull();
});

it("opens one named keyboard reader with complete task identities and literal details", async () => {
  const path = "/workspace/" + "unbroken_source_".repeat(12) + "final.ts";
  startTask("test", { label: "Build " + path, message: "Reading source\n" + path, progress: 0.46 });
  renderTasks();
  await openTasks("Build " + path);
  const popup = screen.getByRole("dialog", { name: "Tasks" });
  const reader = within(popup).getByRole("region", { name: "Tasks" });
  expect(reader.tabIndex).toBe(0);
  expect(within(reader).getByText("Build " + path).textContent).toBe("Build " + path);
  expect(within(reader).getByText("Reading source " + path).textContent).toBe(
    "Reading source\n" + path,
  );
  expect(
    within(reader)
      .getByRole("progressbar", { name: "Build " + path })
      .getAttribute("aria-valuenow"),
  ).toBe("46");
});

it("names running, completed and failed state without using color as the only accessible cue", async () => {
  startTask("test", { label: "Running", progress: null });
  startTask("test", { label: "Completed", progress: 0.5 }).succeed("Index ready");
  startTask("test", { label: "Failed", progress: 0.7 }).fail(new Error("Read permission denied"));
  renderTasks();
  await openTasks("Running");
  expect(screen.getByRole("img", { name: "Running" })).toBeTruthy();
  expect(screen.getByRole("img", { name: "Completed" })).toBeTruthy();
  expect(screen.getByRole("img", { name: "Failed" })).toBeTruthy();
  expect(screen.getByText("Read permission denied")).toBeTruthy();
  expect(screen.queryByRole("progressbar", { name: "Running" })).toBeNull();
  expect(screen.queryByRole("progressbar", { name: "Failed" })).toBeNull();
  expect(screen.getByRole("progressbar", { name: "Completed" }).getAttribute("aria-valuenow")).toBe(
    "100",
  );
});

it("keeps the same reader, native position and later draft focus through progress updates", async () => {
  const task = startTask("test", { label: "Build", message: "First read", progress: 0.1 });
  renderTasks();
  await openTasks("Build");
  const reader = screen.getByRole("region", { name: "Tasks" });
  act(() => reader.focus());
  reader.scrollTop = 80;
  act(() => task.update({ message: "Second read", progress: 0.78 }));
  expect(screen.getByRole("region", { name: "Tasks" })).toBe(reader);
  expect(document.activeElement).toBe(reader);
  expect(reader.scrollTop).toBe(80);
  const draft = screen.getByRole("textbox", { name: "Draft" });
  act(() => draft.focus());
  act(() => task.update({ message: "Third read" }));
  expect(document.activeElement).toBe(draft);
  expect(screen.getByRole("textbox", { name: "Draft" })).toBe(draft);
  expect(screen.getByRole("progressbar", { name: "Build" }).getAttribute("aria-valuenow")).toBe(
    "78",
  );
});
