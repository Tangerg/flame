import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { useState } from "react";
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { SearchOverlay } from "@/ui/atoms/search-overlay";
function Harness({
  onSelect = () => undefined,
  showSource = true,
}: {
  onSelect?: () => void;
  showSource?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      {showSource && (
        <button type="button" onClick={() => setOpen(true)}>
          Find
        </button>
      )}
      <button type="button">Destination</button>
      <SearchOverlay
        open={open}
        onOpenChange={setOpen}
        label="Finder"
        placeholder="Search…"
        empty={<p>nothing</p>}
        options={() => [{ key: "a", onSelect, children: <span>A</span> }]}
      />
    </>
  );
}

afterEach(async () => {
  cleanup();
  await drainBrowserTasks();
});

describe("search overlay", () => {
  it("hands focus back to whatever opened it", async () => {
    testClient.render(<Harness />);
    const trigger = screen.getByRole("button", { name: "Find" });
    trigger.focus();

    trigger.click();
    const field = await screen.findByRole("combobox");
    field.focus();
    expect(document.activeElement).toBe(field);

    screen.getByRole("option").click();

    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });

  it.each(["pointer", "keyboard"])(
    "delivers one %s choice after closing and keeps its destination focused",
    async (method) => {
      const choose = vi.fn(() => {
        expect(screen.queryByRole("dialog")).toBeNull();
        expect(destination.closest('[aria-hidden="true"], [data-base-ui-inert]')).toBeNull();
        destination.focus();
      });
      testClient.render(<Harness onSelect={choose} />);
      const trigger = screen.getByRole("button", { name: "Find" });
      const destination = screen.getByRole("button", { name: "Destination" });
      trigger.focus();
      fireEvent.click(trigger);
      const field = await screen.findByRole("combobox");
      const option = screen.getByRole("option");

      act(() => {
        for (let repeat = 0; repeat < 2; repeat += 1) {
          if (method === "pointer") fireEvent.click(option);
          else fireEvent.keyDown(field, { key: "Enter" });
        }
      });

      await waitFor(() => expect(choose).toHaveBeenCalledOnce());
      await drainBrowserTasks();
      expect(document.activeElement).toBe(destination);
    },
  );

  it("restores context before an action that does not choose another focus target", async () => {
    const choose = vi.fn(() => expect(document.activeElement).toBe(trigger));
    testClient.render(<Harness onSelect={choose} />);
    const trigger = screen.getByRole("button", { name: "Find" });
    trigger.focus();
    fireEvent.click(trigger);
    fireEvent.click(await screen.findByRole("option"));

    await waitFor(() => expect(choose).toHaveBeenCalledOnce());
    await drainBrowserTasks();
    expect(document.activeElement).toBe(trigger);
  });

  it("returns on cancellation without delivering a choice", async () => {
    const choose = vi.fn();
    testClient.render(<Harness onSelect={choose} />);
    const trigger = screen.getByRole("button", { name: "Find" });
    trigger.focus();
    fireEvent.click(trigger);
    fireEvent.keyDown(await screen.findByRole("combobox"), { key: "Escape" });

    await waitFor(() => expect(document.activeElement).toBe(trigger));
    await drainBrowserTasks();
    expect(choose).not.toHaveBeenCalled();
  });

  it("does not repeat a delivered choice on reopen and cancellation", async () => {
    const choose = vi.fn();
    testClient.render(<Harness onSelect={choose} />);
    const trigger = screen.getByRole("button", { name: "Find" });
    trigger.focus();
    fireEvent.click(trigger);
    fireEvent.click(await screen.findByRole("option"));
    await waitFor(() => expect(choose).toHaveBeenCalledOnce());
    await waitFor(() => expect(document.activeElement).toBe(trigger));

    fireEvent.click(trigger);
    const field = await screen.findByRole("combobox");
    fireEvent.keyDown(field, { key: "Escape" });
    await waitFor(() => expect(document.activeElement).toBe(trigger));
    await drainBrowserTasks();
    expect(choose).toHaveBeenCalledOnce();

    fireEvent.click(trigger);
    fireEvent.keyDown(await screen.findByRole("combobox"), { key: "Enter" });
    await waitFor(() => expect(choose).toHaveBeenCalledTimes(2));
  });

  it.each(["cancel", "select"])("respects later outside focus during %s", async (intent) => {
    const choose = vi.fn();
    testClient.render(<Harness onSelect={choose} />);
    const trigger = screen.getByRole("button", { name: "Find" });
    const destination = screen.getByRole("button", { name: "Destination" });
    trigger.focus();
    fireEvent.click(trigger);
    const field = await screen.findByRole("combobox");
    if (intent === "cancel") fireEvent.keyDown(field, { key: "Escape" });
    else fireEvent.click(screen.getByRole("option"));
    destination.focus();
    expect(document.activeElement).toBe(destination);

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await drainBrowserTasks();
    expect(document.activeElement).toBe(destination);
    expect(choose).toHaveBeenCalledTimes(intent === "select" ? 1 : 0);
  });

  it("can deliver a destination after its opener disappears", async () => {
    const choose = vi.fn(() => destination.focus());
    const { rerender } = testClient.render(<Harness onSelect={choose} />);
    const trigger = screen.getByRole("button", { name: "Find" });
    const destination = screen.getByRole("button", { name: "Destination" });
    trigger.focus();
    fireEvent.click(trigger);
    await screen.findByRole("combobox");
    rerender(<Harness onSelect={choose} showSource={false} />);
    expect(trigger.isConnected).toBe(false);
    fireEvent.click(screen.getByRole("option"));

    await waitFor(() => expect(choose).toHaveBeenCalledOnce());
    await drainBrowserTasks();
    expect(document.activeElement).toBe(destination);
  });

  it("retires an undelivered choice when the surface unmounts", async () => {
    const choose = vi.fn();
    const { unmount } = testClient.render(<Harness onSelect={choose} />);
    const trigger = screen.getByRole("button", { name: "Find" });
    trigger.focus();
    fireEvent.click(trigger);
    fireEvent.click(await screen.findByRole("option"));
    unmount();

    await drainBrowserTasks();
    expect(choose).not.toHaveBeenCalled();
  });
});
