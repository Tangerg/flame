import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, onTestFinished, vi } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { drainBrowserTasks } from "@/test/browserTasks";
import { TextEditorDialog } from "@/ui/atoms/text-editor-dialog";

let testClient: TestClient;
beforeEach(() => {
  const owned = createTestClient();
  testClient = owned;
  onTestFinished(() => owned.dispose());
});
afterEach(async () => {
  cleanup();
  await drainBrowserTasks();
});

function editor(busy: boolean, onOpenChange: (open: boolean) => void) {
  return (
    <TextEditorDialog
      open
      onOpenChange={onOpenChange}
      title="Edit captured objective"
      closeLabel="Close"
      label="Objective"
      value="Keep this exact draft."
      onChange={vi.fn()}
      cancelLabel="Cancel"
      saveLabel="Save"
      savingLabel="Saving"
      busy={busy}
      onSave={vi.fn()}
    />
  );
}

it("keeps the close action unavailable with the other pending actions", async () => {
  const change = vi.fn();
  const view = testClient.render(editor(true, change));
  const close = await screen.findByRole<HTMLButtonElement>("button", { name: "Close" });

  expect(close.disabled).toBe(true);
  expect(screen.getByRole("button", { name: "Cancel" }).getAttribute("aria-disabled")).toBe("true");
  fireEvent.click(close);
  expect(change).not.toHaveBeenCalled();

  view.rerender(editor(false, change));
  expect(close.disabled).toBe(false);
  fireEvent.click(close);
  expect(change).toHaveBeenLastCalledWith(false);
});

it("keeps Escape inside the saving task and restores dismissal after settlement", async () => {
  const change = vi.fn();
  const view = testClient.render(editor(true, change));
  const input = await screen.findByRole<HTMLTextAreaElement>("textbox", { name: "Objective" });
  input.focus();

  fireEvent.keyDown(input, { key: "Escape" });
  expect(change).not.toHaveBeenCalled();
  expect(input.value).toBe("Keep this exact draft.");

  view.rerender(editor(false, change));
  fireEvent.keyDown(input, { key: "Escape" });
  expect(change).toHaveBeenLastCalledWith(false);
});
