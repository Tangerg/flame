import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { ContentVisibility } from "@/ui/content-visibility";
import { LightboxDialog } from "@/ui/atoms/lightbox-dialog";

function Harness({
  visible,
  finalFocus,
  onOpenChange,
  kind = "figure",
}: {
  visible: boolean;
  finalFocus?: boolean;
  onOpenChange: (open: boolean) => void;
  kind?: "figure" | "media" | "document";
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <ContentVisibility visible={visible}>
        <LightboxDialog
          open={open}
          onOpenChange={(next) => {
            onOpenChange(next);
            setOpen(next);
          }}
          finalFocus={finalFocus}
          {...(kind === "document"
            ? { kind, closeLabel: "Close", contentLabel: "Preview content" }
            : { kind })}
          title="Preview"
          trigger={<button>Expand</button>}
        >
          <p>Material</p>
        </LightboxDialog>
      </ContentVisibility>
      <button>Later control</button>
    </>
  );
}

async function settle() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

async function openLightbox() {
  const trigger = screen.getByRole("button", { name: "Expand" });
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const dialog = screen.getByRole("dialog", { name: "Preview" });
  await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
  return { trigger, dialog };
}

afterEach(cleanup);

it.each(["figure", "media", "document"] as const)(
  "retires %s once and stays closed when the content returns",
  async (kind) => {
    const onOpenChange = vi.fn();
    const view = render(<Harness visible kind={kind} onOpenChange={onOpenChange} />);
    const { trigger } = await openLightbox();
    expect(onOpenChange).toHaveBeenLastCalledWith(true);

    view.rerender(<Harness visible={false} kind={kind} onOpenChange={onOpenChange} />);
    await settle();
    expect(screen.queryByRole("dialog", { name: "Preview" })).toBeNull();
    expect(onOpenChange).toHaveBeenCalledTimes(2);
    expect(onOpenChange).toHaveBeenLastCalledWith(false);
    expect(document.activeElement).toBe(document.body);

    view.rerender(<Harness visible kind={kind} onOpenChange={onOpenChange} />);
    await settle();
    expect(screen.queryByRole("dialog", { name: "Preview" })).toBeNull();
    expect(onOpenChange).toHaveBeenCalledTimes(2);
    expect(document.activeElement).not.toBe(trigger);

    fireEvent.click(trigger);
    expect(screen.getByRole("dialog", { name: "Preview" })).toBeTruthy();
  },
);

it("leaves an unrelated control focused when its content hides", async () => {
  const onOpenChange = vi.fn();
  const view = render(<Harness visible onOpenChange={onOpenChange} />);
  const later = screen.getByRole("button", { name: "Later control" });
  await openLightbox();
  act(() => later.focus());

  view.rerender(<Harness visible={false} onOpenChange={onOpenChange} />);
  await settle();
  expect(screen.queryByRole("dialog", { name: "Preview" })).toBeNull();
  expect(document.activeElement).toBe(later);
});

it("returns an ordinary visible close to its trigger", async () => {
  render(<Harness visible onOpenChange={() => {}} />);
  const { trigger, dialog } = await openLightbox();

  fireEvent.keyDown(dialog, { key: "Escape" });
  await settle();
  expect(screen.queryByRole("dialog", { name: "Preview" })).toBeNull();
  expect(document.activeElement).toBe(trigger);
});

it("keeps a caller that declines final focus away from the trigger", async () => {
  render(<Harness visible finalFocus={false} onOpenChange={() => {}} />);
  const { trigger, dialog } = await openLightbox();

  fireEvent.keyDown(dialog, { key: "Escape" });
  await settle();
  expect(screen.queryByRole("dialog", { name: "Preview" })).toBeNull();
  expect(document.activeElement).not.toBe(trigger);
});
