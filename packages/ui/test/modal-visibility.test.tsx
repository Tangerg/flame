import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { useRef, useState, type ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ContentVisibility, ModalDialog, Select } from "../dist/index.js";

type FinalFocus = ComponentProps<typeof ModalDialog>["finalFocus"];

beforeEach(() => {
  // Base UI's clipping signal needs visible anchor geometry in layout-free Happy DOM.
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    function (this: HTMLElement) {
      if (this.closest("[hidden]")) return new DOMRect();
      return this === document.documentElement || this === document.body
        ? new DOMRect(0, 0, 1024, 768)
        : new DOMRect(20, 20, 160, 32);
    },
  );
  vi.spyOn(document.documentElement, "clientWidth", "get").mockReturnValue(
    1024,
  );
  vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(
    768,
  );
  vi.spyOn(HTMLElement.prototype, "getClientRects").mockImplementation(
    function (this: HTMLElement) {
      return (this.closest("[hidden]") ? [] : [{}]) as unknown as DOMRectList;
    },
  );
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const options = [
  { value: "fast", label: "Fast" },
  { value: "deep", label: "Deep" },
];

function Scene({
  visible,
  finalFocus,
  dismissible = true,
  withFallback = true,
  innerHidden = false,
  alternateFallback = false,
}: {
  visible: boolean;
  finalFocus?: FinalFocus;
  dismissible?: boolean;
  withFallback?: boolean;
  innerHidden?: boolean;
  alternateFallback?: boolean;
}) {
  const fallback = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [model, setModel] = useState("fast");
  return (
    <>
      <button ref={alternateFallback ? null : fallback}>Conversation</button>
      <button ref={alternateFallback ? fallback : null}>Other material</button>
      <button>Outside</button>
      <div hidden={!visible}>
        <ContentVisibility
          visible={visible}
          fallbackFocus={withFallback ? fallback : undefined}
        >
          <button onClick={() => setOpen(true)}>Add schedule</button>
          <ModalDialog
            open={open}
            onOpenChange={setOpen}
            title="Add schedule"
            finalFocus={finalFocus}
            dismissible={dismissible}
            footer={<button onClick={() => setOpen(false)}>Cancel</button>}
          >
            <input
              aria-label="Title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
            <ContentVisibility visible={!innerHidden}>
              <Select
                label="Model"
                value={model}
                onValueChange={setModel}
                options={options}
              />
            </ContentVisibility>
          </ModalDialog>
        </ContentVisibility>
      </div>
    </>
  );
}

async function openDialog(props: Partial<ComponentProps<typeof Scene>> = {}) {
  const view = render(<Scene visible {...props} />);
  const opener = screen.getByRole("button", { name: "Add schedule" });
  act(() => opener.focus());
  fireEvent.click(opener);
  const dialog = await screen.findByRole("dialog");
  fireEvent.change(screen.getByRole("textbox", { name: "Title" }), {
    target: { value: "Nightly" },
  });
  return {
    dialog,
    opener,
    hide: () => view.rerender(<Scene {...props} visible={false} />),
    reveal: () => view.rerender(<Scene {...props} visible />),
    showOtherMaterial: () =>
      view.rerender(<Scene {...props} visible={false} alternateFallback />),
  };
}

async function openSelect() {
  const trigger = screen.getByRole("combobox", { name: "Model" });
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const listbox = await screen.findByRole("listbox");
  await vi.waitFor(() =>
    expect(listbox.contains(document.activeElement)).toBe(true),
  );
  return listbox;
}

function pressEscape() {
  fireEvent.keyDown(document.activeElement ?? document.body, {
    key: "Escape",
  });
}

describe("ModalDialog over a hidden owner", () => {
  it("keeps the dialog, draft and open Select through hide and reveal", async () => {
    const scene = await openDialog();
    const listbox = await openSelect();
    const focused = document.activeElement;

    scene.hide();

    expect(screen.getByRole("dialog")).toBe(scene.dialog);
    expect(screen.getByRole("listbox")).toBe(listbox);
    expect(document.activeElement).toBe(focused);
    expect(screen.getByRole("textbox", { name: "Title" })).toHaveProperty(
      "value",
      "Nightly",
    );

    scene.reveal();

    expect(screen.getByRole("dialog")).toBe(scene.dialog);
    expect(screen.getByRole("listbox")).toBe(listbox);
  });

  it("closes the nested popup on Escape before the dialog", async () => {
    const scene = await openDialog();
    await openSelect();
    scene.hide();

    pressEscape();
    await vi.waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    expect(screen.getByRole("dialog")).toBe(scene.dialog);

    pressEscape();
    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("returns focus to the host fallback instead of the hidden opener", async () => {
    const scene = await openDialog();
    scene.hide();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await vi.waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Conversation" }),
      ),
    );
    expect(document.activeElement).not.toBe(scene.opener);
  });

  it("does not focus the hidden opener without a fallback", async () => {
    const scene = await openDialog({ withFallback: false });
    scene.hide();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).not.toBe(scene.opener);
  });

  it("resolves the current fallback target only when the task closes", async () => {
    const scene = await openDialog();
    scene.hide();
    scene.showOtherMaterial();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await vi.waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Other material" }),
      ),
    );
  });

  it("passes a visible caller's resolver through unchanged", async () => {
    const resolve = vi.fn(() => true);
    const scene = await openDialog({ finalFocus: resolve });

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await vi.waitFor(() => expect(document.activeElement).toBe(scene.opener));
    expect(resolve).toHaveBeenCalledTimes(1);
  });

  it("honors do-nothing and visible explicit targets while hidden", async () => {
    const none = vi.fn(() => undefined);
    const first = await openDialog({ finalFocus: none });
    first.hide();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(none).toHaveBeenCalledTimes(1);
    expect(document.activeElement).not.toBe(
      screen.getByRole("button", { name: "Conversation" }),
    );
    cleanup();

    const second = await openDialog({ finalFocus: false });
    second.hide();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).not.toBe(
      screen.getByRole("button", { name: "Conversation" }),
    );
    cleanup();

    const outside = vi.fn(() =>
      screen.getByRole("button", { name: "Outside" }),
    );
    const third = await openDialog({ finalFocus: outside });
    third.hide();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await vi.waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Outside" }),
      ),
    );
    expect(outside).toHaveBeenCalledTimes(1);
  });

  it("lets a hidden provider inside the dialog still retire its popup", async () => {
    const view = render(<Scene visible />);
    fireEvent.click(screen.getByRole("button", { name: "Add schedule" }));
    await screen.findByRole("dialog");
    await openSelect();

    view.rerender(<Scene visible innerHidden />);

    await vi.waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("keeps a non-dismissible dialog open while its owner is hidden", async () => {
    const scene = await openDialog({ dismissible: false });
    scene.hide();

    act(() => scene.dialog.focus());
    pressEscape();

    expect(screen.getByRole("dialog")).toBe(scene.dialog);
  });
});
