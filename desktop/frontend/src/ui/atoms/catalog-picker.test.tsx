import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CatalogPicker } from "./catalog-picker";

afterEach(cleanup);

it("delivers a selection after the catalog closes so its consumer can create a native surface", async () => {
  const stillOpen = vi.fn();
  const item = { id: "trajectory", label: "Trajectory" };
  render(
    <CatalogPicker
      label="Browse panels"
      placeholder="Search panels"
      emptyLabel="No panels"
      groups={[{ id: "panels", label: "Panels", items: [item] }]}
      onSelect={(selected) => {
        expect(selected).toEqual(item);
        stillOpen(document.querySelector("[data-base-ui-portal] [data-open]") !== null);
      }}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Browse panels" }));
  fireEvent.click(await screen.findByRole("option", { name: "Trajectory" }));
  await vi.waitFor(() => expect(stillOpen).toHaveBeenCalledExactlyOnceWith(false));
});
