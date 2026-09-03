import { useState } from "react";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { SearchOverlay } from "./search-overlay";

function Harness() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Find
      </button>
      <SearchOverlay
        open={open}
        onOpenChange={setOpen}
        label="Finder"
        placeholder="Search…"
        empty={<p>nothing</p>}
        options={() => [{ key: "a", onSelect: () => setOpen(false), children: <span>A</span> }]}
      />
    </>
  );
}

afterEach(async () => {
  cleanup();
  await drainBrowserTasks();
});

describe("search overlay", () => {
  // A controlled dialog has no trigger node for Base UI to restore focus to, so without this
  // the caller lands on <body> and the next key press goes nowhere.
  it("hands focus back to whatever opened it", async () => {
    render(<Harness />);
    const trigger = screen.getByRole("button", { name: "Find" });
    trigger.focus();

    trigger.click();
    const field = await screen.findByRole("combobox");
    field.focus();
    expect(document.activeElement).toBe(field);

    screen.getByRole("option").click();

    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});
