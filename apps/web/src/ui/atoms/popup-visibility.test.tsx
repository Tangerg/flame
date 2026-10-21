import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { createRef, useRef, useState, type ComponentProps, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ContentVisibility, ContextMenu, DropdownMenu, Popover } from "@/ui/index";
type PopoverOpenChange = NonNullable<ComponentProps<typeof Popover.Root>["onOpenChange"]>;

type MenuOpenChange = NonNullable<ComponentProps<typeof DropdownMenu.Root>["onOpenChange"]>;

type ContextOpenChange = NonNullable<ComponentProps<typeof ContextMenu.Root>["onOpenChange"]>;

type MenuActions = NonNullable<ComponentProps<typeof DropdownMenu.Root>["actionsRef"]>;

afterEach(cleanup);

function Host({ visible, children }: { visible: boolean; children: ReactNode }) {
  return (
    <>
      <button>Outside</button>
      <div hidden={!visible}>
        <ContentVisibility visible={visible}>{children}</ContentVisibility>
      </div>
    </>
  );
}

function mount(popup: ReactNode) {
  const view = render(<Host visible>{popup}</Host>);
  return {
    hide: () => view.rerender(<Host visible={false}>{popup}</Host>),
    reveal: () => view.rerender(<Host visible>{popup}</Host>),
  };
}

function LineField() {
  return <input aria-label="Line" />;
}

function ControlledPopover({ onOpenChange }: { onOpenChange: PopoverOpenChange }) {
  const [open, setOpen] = useState(false);
  return (
    <Popover.Root
      open={open}
      onOpenChange={(next, details) => {
        onOpenChange(next, details);
        setOpen(next);
      }}
    >
      <Popover.Trigger>Go</Popover.Trigger>
      <Popover.Content aria-label="Go panel">
        <LineField />
      </Popover.Content>
    </Popover.Root>
  );
}

function UncontrolledPopover({ onOpenChange }: { onOpenChange: PopoverOpenChange }) {
  return (
    <Popover.Root onOpenChange={onOpenChange}>
      <Popover.Trigger>Go</Popover.Trigger>
      <Popover.Content aria-label="Go panel">
        <LineField />
      </Popover.Content>
    </Popover.Root>
  );
}

function popoverOf(mode: "controlled" | "uncontrolled", onOpenChange: PopoverOpenChange) {
  return mode === "controlled" ? (
    <ControlledPopover onOpenChange={onOpenChange} />
  ) : (
    <UncontrolledPopover onOpenChange={onOpenChange} />
  );
}

async function openPopover() {
  fireEvent.click(screen.getByRole("button", { name: "Go" }));
  const dialog = await screen.findByRole("dialog", { name: "Go panel" });
  const field = within(dialog).getByRole("textbox", { name: "Line" });
  act(() => field.focus());
  return { dialog, field };
}

const imperativeClose = expect.objectContaining({ reason: "imperative-action" });

describe("Popover retirement", () => {
  it.each(["controlled", "uncontrolled"] as const)(
    "closes a %s popover when its host hides and does not bring it back on reveal",
    async (mode) => {
      const onOpenChange = vi.fn<PopoverOpenChange>();
      const host = mount(popoverOf(mode, onOpenChange));
      const { dialog } = await openPopover();
      onOpenChange.mockClear();

      host.hide();

      expect(document.activeElement).toBe(document.body);
      expect(onOpenChange).toHaveBeenCalledOnce();
      expect(onOpenChange).toHaveBeenCalledWith(false, imperativeClose);
      await vi.waitFor(() => expect(dialog.isConnected).toBe(false));

      host.reveal();
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(onOpenChange).toHaveBeenCalledOnce();

      await openPopover();
      expect(onOpenChange).toHaveBeenLastCalledWith(true, expect.anything());
    },
  );

  it.each(["controlled", "uncontrolled"] as const)(
    "sends no close to a %s popover that is already closed",
    async (mode) => {
      const onOpenChange = vi.fn<PopoverOpenChange>();
      const host = mount(popoverOf(mode, onOpenChange));
      host.hide();
      host.reveal();
      expect(onOpenChange).not.toHaveBeenCalled();

      const { dialog, field } = await openPopover();
      fireEvent.keyDown(field, { key: "Escape" });
      await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
      expect(onOpenChange).toHaveBeenLastCalledWith(
        false,
        expect.objectContaining({ reason: "escape-key" }),
      );
      const trigger = screen.getByRole("button", { name: "Go" });
      await vi.waitFor(() => expect(document.activeElement).toBe(trigger));
      onOpenChange.mockClear();

      host.hide();
      host.reveal();

      expect(onOpenChange).not.toHaveBeenCalled();
      expect(screen.queryByRole("dialog")).toBeNull();
    },
  );

  it("retires a popover whose caller canceled an earlier dismissal", async () => {
    const onOpenChange = vi.fn<PopoverOpenChange>((_, details) => {
      if (details.reason === "escape-key") details.cancel();
    });
    const host = mount(<UncontrolledPopover onOpenChange={onOpenChange} />);
    const { dialog, field } = await openPopover();
    fireEvent.keyDown(field, { key: "Escape" });
    expect(dialog.isConnected).toBe(true);

    host.hide();

    expect(onOpenChange).toHaveBeenLastCalledWith(false, imperativeClose);
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
  });

  it("sends no close when the caller canceled the opening", () => {
    const onOpenChange = vi.fn<PopoverOpenChange>((open, details) => {
      if (open) details.cancel();
    });
    const host = mount(<UncontrolledPopover onOpenChange={onOpenChange} />);
    fireEvent.click(screen.getByRole("button", { name: "Go" }));
    expect(screen.queryByRole("dialog")).toBeNull();

    host.hide();

    expect(onOpenChange).toHaveBeenCalledOnce();
    expect(onOpenChange).toHaveBeenCalledWith(true, expect.anything());
  });

  it("keeps focus that moved outside while the host hides", async () => {
    const host = mount(<ControlledPopover onOpenChange={() => {}} />);
    const { dialog } = await openPopover();
    const outside = screen.getByRole("button", { name: "Outside" });

    act(() => {
      host.hide();
      outside.focus();
    });
    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));

    expect(document.activeElement).toBe(outside);
  });

  it("retires an anchored panel without moving the focus it never owned", async () => {
    const onOpenChange = vi.fn<(open: boolean) => void>();
    function Suggestions() {
      const anchor = useRef<HTMLTextAreaElement>(null);
      const [open, setOpen] = useState(true);
      return (
        <>
          <textarea ref={anchor} aria-label="Draft" />
          <Popover.Anchored
            open={open}
            anchor={anchor}
            onOpenChange={(next) => {
              onOpenChange(next);
              setOpen(next);
            }}
            role="listbox"
            aria-label="Suggestions"
          >
            <div role="option" aria-selected={false}>
              first
            </div>
          </Popover.Anchored>
        </>
      );
    }
    const host = mount(<Suggestions />);
    const panel = await screen.findByRole("listbox", { name: "Suggestions" });
    const draft = screen.getByRole("textbox", { name: "Draft" });
    act(() => draft.focus());

    host.hide();

    expect(onOpenChange).toHaveBeenCalledOnce();
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(document.activeElement).toBe(draft);
    await vi.waitFor(() => expect(panel.isConnected).toBe(false));
    host.reveal();
    expect(screen.queryByRole("listbox", { name: "Suggestions" })).toBeNull();
  });
});

describe("menu retirement", () => {
  function FileMenu({
    onOpenChange,
    onCopy,
  }: {
    onOpenChange: ContextOpenChange;
    onCopy: () => void;
  }) {
    return (
      <ContextMenu.Root onOpenChange={onOpenChange}>
        <ContextMenu.Trigger render={<button>apps</button>} />
        <ContextMenu.Content>
          <ContextMenu.IconItem icon="copy" onSelect={onCopy}>
            Copy path
          </ContextMenu.IconItem>
        </ContextMenu.Content>
      </ContextMenu.Root>
    );
  }

  async function openFileMenu() {
    const row = screen.getByRole("button", { name: "apps" });
    act(() => row.focus());
    fireEvent.contextMenu(row);
    const menu = await screen.findByRole("menu");
    await vi.waitFor(() => expect(menu.contains(document.activeElement)).toBe(true));
    return menu;
  }

  it("closes an open context menu and releases its focus when the host hides", async () => {
    const onOpenChange = vi.fn<ContextOpenChange>();
    const host = mount(<FileMenu onOpenChange={onOpenChange} onCopy={() => {}} />);
    const menu = await openFileMenu();

    host.hide();

    expect(document.activeElement).toBe(document.body);
    expect(onOpenChange).toHaveBeenLastCalledWith(false, imperativeClose);
    await vi.waitFor(() => expect(menu.isConnected).toBe(false));
    host.reveal();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("delivers a visible selection and sends no later close", async () => {
    const onOpenChange = vi.fn<ContextOpenChange>();
    const onCopy = vi.fn();
    const host = mount(<FileMenu onOpenChange={onOpenChange} onCopy={onCopy} />);
    const menu = await openFileMenu();

    fireEvent.click(within(menu).getByRole("menuitem", { name: "Copy path" }));
    await vi.waitFor(() => expect(menu.isConnected).toBe(false));
    expect(onCopy).toHaveBeenCalledOnce();
    expect(onOpenChange).toHaveBeenLastCalledWith(
      false,
      expect.objectContaining({ reason: "item-press" }),
    );
    onOpenChange.mockClear();

    host.hide();
    host.reveal();

    expect(onOpenChange).not.toHaveBeenCalled();
  });

  function ActionsMenu({
    actionsRef,
    onOpenChange,
  }: {
    actionsRef: MenuActions;
    onOpenChange: MenuOpenChange;
  }) {
    return (
      <DropdownMenu.Root defaultOpen actionsRef={actionsRef} onOpenChange={onOpenChange}>
        <DropdownMenu.Trigger>Actions</DropdownMenu.Trigger>
        <DropdownMenu.Content>
          <DropdownMenu.Item>Rename</DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    );
  }

  it("retires a menu opened by default through the caller's own actions", async () => {
    const actions: MenuActions = createRef();
    const onOpenChange = vi.fn<MenuOpenChange>();
    const host = mount(<ActionsMenu actionsRef={actions} onOpenChange={onOpenChange} />);
    const menu = await screen.findByRole("menu");
    expect(actions.current).not.toBeNull();

    host.hide();

    expect(onOpenChange).toHaveBeenCalledOnce();
    expect(onOpenChange).toHaveBeenCalledWith(false, imperativeClose);
    await vi.waitFor(() => expect(menu.isConnected).toBe(false));
    host.reveal();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("keeps the caller's close action working while visible", async () => {
    const actions: MenuActions = createRef();
    const onOpenChange = vi.fn<MenuOpenChange>();
    const host = mount(<ActionsMenu actionsRef={actions} onOpenChange={onOpenChange} />);
    const menu = await screen.findByRole("menu");

    act(() => actions.current!.close());

    expect(onOpenChange).toHaveBeenCalledOnce();
    expect(onOpenChange).toHaveBeenCalledWith(false, imperativeClose);
    await vi.waitFor(() => expect(menu.isConnected).toBe(false));
    host.hide();
    expect(onOpenChange).toHaveBeenCalledOnce();
  });
});
