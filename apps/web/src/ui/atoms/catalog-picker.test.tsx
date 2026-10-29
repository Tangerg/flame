import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { createRef } from "react";
import { CatalogList, CatalogPicker, RailCatalogPicker } from "@/ui/atoms/catalog-picker";
afterEach(cleanup);

it("keeps feedback outside options and preserves focus and provider scope when a catalog arrives", async () => {
  const choose = vi.fn();
  const groups = [
    { id: "a", label: "Provider A", items: [{ id: "a/model", label: "First model" }] },
    { id: "b", label: "Provider B", items: [{ id: "b/model", label: "Kept model", active: true }] },
  ];
  function picker(loaded: boolean, preferred = "b") {
    return (
      <RailCatalogPicker
        label="Switch model"
        placeholder="Search models"
        emptyLabel={loaded ? "No models" : "Loading"}
        clearLabel="Clear search"
        onSelect={choose}
        groups={loaded ? groups : []}
        openAtGroupId={preferred}
        feedback={!loaded ? <div role="alert">The catalog could not be read.</div> : undefined}
        footer={<button type="button">Refresh models</button>}
      />
    );
  }
  const view = testClient.render(picker(false));
  fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
  const search = await screen.findByRole("combobox", { name: "Search models" });
  expect(screen.getByRole("alert").closest('[role="listbox"]')).toBeNull();
  const refresh = screen.getByRole("button", { name: "Refresh models" });
  act(() => refresh.focus());
  view.rerender(picker(true));
  expect(screen.getByRole("combobox", { name: "Search models" })).toBe(search);
  expect(screen.getByRole("button", { name: "Refresh models" })).toBe(refresh);
  expect(document.activeElement).toBe(refresh);
  expect(screen.getByRole("button", { name: "Provider B", pressed: true })).toBeTruthy();
  expect(await screen.findByRole("option", { name: "Kept model" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Provider A" }));
  view.rerender(picker(true, "b"));
  expect(screen.getByRole("button", { name: "Provider A", pressed: true })).toBeTruthy();
  expect(screen.getByRole("option", { name: "First model" })).toBeTruthy();
  expect(choose).not.toHaveBeenCalled();
});

it.each(["pointer", "Escape"])(
  "clears from its %s action and keeps keyboard selection",
  async (action) => {
    const select = vi.fn();
    const inputRef = createRef<HTMLInputElement>();
    const child = { id: "child", label: "Subagents" };
    testClient.render(
      <CatalogList
        label="Panels"
        placeholder="Find a panel"
        emptyLabel="No panels"
        clearLabel="Clear search"
        inputRef={inputRef}
        groups={[{ id: "panels", label: "Panels", items: [{ id: "file", label: "Files" }, child] }]}
        onSelect={select}
      />,
    );
    const search = screen.getByRole<HTMLInputElement>("combobox", { name: "Find a panel" });
    expect(inputRef.current).toBe(search);
    expect(search.type).toBe("search");
    expect(search.getAttribute("aria-expanded")).toBe("true");
    expect(document.getElementById(search.getAttribute("aria-controls") ?? "")).toBe(
      screen.getByRole("listbox"),
    );
    fireEvent.change(search, { target: { value: "no-match" } });
    await screen.findByText("No panels");
    const clear = screen.getByRole("button", { name: "Clear search" });
    clear.focus();
    if (action === "pointer") fireEvent.click(clear);
    else fireEvent.keyDown(clear, { key: "Escape" });
    expect(search.value).toBe("");
    expect(document.activeElement).toBe(search);
    expect(select).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
    fireEvent.change(search, { target: { value: "Sub" } });
    await screen.findByRole("option", { name: "Subagents" });
    fireEvent.keyDown(search, { key: "ArrowDown" });
    fireEvent.keyDown(search, { key: "Enter" });
    await vi.waitFor(() => expect(select).toHaveBeenCalledExactlyOnceWith(child));
  },
);

it.each([{ isComposing: true }, { keyCode: 229 }])(
  "leaves the catalog query with the composing input for %j",
  (composition) => {
    testClient.render(
      <CatalogList
        label="Panels"
        placeholder="Find a panel"
        emptyLabel="No panels"
        clearLabel="Clear search"
        groups={[{ id: "panels", label: "Panels", items: [{ id: "file", label: "Files" }] }]}
        onSelect={() => {}}
      />,
    );
    const search = screen.getByRole<HTMLInputElement>("combobox", { name: "Find a panel" });
    search.focus();
    fireEvent.change(search, { target: { value: "Files" } });
    fireEvent.keyDown(search, { key: "Escape", ...composition });
    expect(search.value).toBe("Files");
    expect(document.activeElement).toBe(search);
  },
);

it("clears a popup query before dismissing and restoring its trigger", async () => {
  const select = vi.fn();
  testClient.render(
    <CatalogPicker
      label="Browse panels"
      placeholder="Find a panel"
      emptyLabel="No panels"
      clearLabel="Clear search"
      groups={[{ id: "panels", label: "Panels", items: [{ id: "file", label: "Files" }] }]}
      onSelect={select}
    />,
  );
  const browse = screen.getByRole("button", { name: "Browse panels" });
  fireEvent.click(browse);
  const search = await screen.findByRole<HTMLInputElement>("combobox", { name: "Find a panel" });
  search.focus();
  fireEvent.change(search, { target: { value: "Files" } });
  fireEvent.keyDown(search, { key: "Escape" });
  expect(screen.getByRole("combobox", { name: "Find a panel" })).toBe(search);
  expect(search.value).toBe("");
  expect(document.activeElement).toBe(search);
  fireEvent.keyDown(search, { key: "Escape" });
  await vi.waitFor(() => expect(screen.queryByRole("combobox")).toBeNull());
  expect(document.activeElement).toBe(browse);
  expect(select).not.toHaveBeenCalled();
});

it("returns a cleared model search to its chosen group and exact item", async () => {
  const select = vi.fn();
  const selected = { id: "b/shared", label: "Shared model", active: true };
  testClient.render(
    <RailCatalogPicker
      label="Switch model"
      placeholder="Search models"
      emptyLabel="No models"
      clearLabel="Clear search"
      openAtGroupId="b"
      groups={[
        { id: "a", label: "Provider A", items: [{ id: "a/only", label: "A only" }] },
        { id: "b", label: "Provider B", items: [selected] },
      ]}
      onSelect={select}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
  const search = await screen.findByRole<HTMLInputElement>("combobox", { name: "Search models" });
  search.focus();
  fireEvent.change(search, { target: { value: "A only" } });
  await screen.findByRole("option", { name: "A only" });
  expect(screen.queryByRole("button", { name: "Provider B" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
  expect(document.activeElement).toBe(search);
  expect(search.value).toBe("");
  expect(screen.getByRole("button", { name: "Provider B" }).getAttribute("aria-pressed")).toBe(
    "true",
  );
  expect(screen.queryByRole("option", { name: "A only" })).toBeNull();
  expect(select).not.toHaveBeenCalled();
  fireEvent.keyDown(search, { key: "ArrowDown" });
  fireEvent.keyDown(search, { key: "Enter" });
  await vi.waitFor(() => expect(select).toHaveBeenCalledExactlyOnceWith(selected));
});

it("clears a catalog query before an enclosing Escape navigation", async () => {
  const leave = vi.fn();
  const observeNavigation = (event: KeyboardEvent) => {
    if (event.key === "Escape") leave();
  };
  document.body.addEventListener("keydown", observeNavigation);
  try {
    testClient.render(
      <CatalogList
        label="Panels"
        placeholder="Find a panel"
        emptyLabel="No panels"
        clearLabel="Clear search"
        groups={[{ id: "workspace", label: "Workspace", items: [{ id: "file", label: "Files" }] }]}
        onSelect={() => {}}
      />,
    );
    const search = screen.getByRole<HTMLInputElement>("combobox", { name: "Find a panel" });
    search.focus();
    fireEvent.change(search, { target: { value: "Files" } });
    fireEvent.keyDown(search, { key: "Escape" });
    await vi.waitFor(() => expect(search.value).toBe(""));
    expect(leave).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(search);
    fireEvent.keyDown(search, { key: "Escape" });
    expect(leave).toHaveBeenCalledOnce();
  } finally {
    document.body.removeEventListener("keydown", observeNavigation);
  }
});

it("filters the inline catalog by keywords, exposes empty results and selects with the keyboard", async () => {
  const select = vi.fn();
  const file = { id: "file", label: "Files", keywords: ["workspace"] };
  testClient.render(
    <CatalogList
      label="Panels"
      placeholder="Find a panel"
      emptyLabel="No panels"
      clearLabel="Clear search"
      groups={[
        { id: "workspace", label: "Workspace", items: [file] },
        { id: "session", label: "Session", items: [{ id: "child", label: "Subagents" }] },
      ]}
      onSelect={select}
    />,
  );
  const search = screen.getByRole("combobox", { name: "Find a panel" });
  fireEvent.change(search, { target: { value: "no-match" } });
  expect(await screen.findByText("No panels")).toBeTruthy();
  expect(screen.queryByRole("option")).toBeNull();

  fireEvent.change(search, { target: { value: "workspace" } });
  expect(await screen.findByRole("option", { name: "Files" })).toBeTruthy();
  expect(screen.queryByRole("option", { name: "Subagents" })).toBeNull();
  search.focus();
  fireEvent.keyDown(search, { key: "ArrowDown" });
  fireEvent.keyDown(search, { key: "Enter" });
  await vi.waitFor(() => expect(select).toHaveBeenCalledExactlyOnceWith(file));
});

it("delivers a selection after the catalog closes so its consumer can create a native surface", async () => {
  const stillOpen = vi.fn();
  const item = { id: "trajectory", label: "Trajectory" };
  testClient.render(
    <>
      <CatalogPicker
        label="Browse panels"
        placeholder="Search panels"
        emptyLabel="No panels"
        clearLabel="Clear search"
        groups={[{ id: "panels", label: "Panels", items: [item] }]}
        onSelect={(selected) => {
          expect(selected).toEqual(item);
          stillOpen(document.querySelector("[data-base-ui-portal] [data-open]") !== null);
          screen.getByRole("button", { name: "Destination" }).focus();
        }}
      />
      <button>Destination</button>
    </>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Browse panels" }));
  fireEvent.click(await screen.findByRole("option", { name: "Trajectory" }));
  await vi.waitFor(() => expect(stillOpen).toHaveBeenCalledExactlyOnceWith(false));
  await act(async () => Promise.resolve());
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Destination" }));

  const browse = screen.getByRole("button", { name: "Browse panels" });
  fireEvent.click(browse);
  fireEvent.keyDown(await screen.findByRole("combobox", { name: "Search panels" }), {
    key: "Escape",
  });
  await vi.waitFor(() => expect(document.activeElement).toBe(browse));
});
