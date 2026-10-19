import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CatalogList, CatalogPicker } from "./catalog-picker";

afterEach(cleanup);

it("filters the inline catalog by keywords, exposes empty results and selects with the keyboard", async () => {
  const select = vi.fn();
  const file = { id: "file", label: "Files", keywords: ["workspace"] };
  render(
    <CatalogList
      label="Panels"
      placeholder="Find a panel"
      emptyLabel="No panels"
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
  render(
    <>
      <CatalogPicker
        label="Browse panels"
        placeholder="Search panels"
        emptyLabel="No panels"
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
