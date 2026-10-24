import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { type ComponentProps } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { writeToClipboard } from "@/lib/clipboard";
import { drainBrowserTasks } from "@/test/browserTasks";
import { ContentVisibility } from "@/ui/index";
import { SessionIdentity } from "@/features/shell/workbench/panel/SessionIdentity";

vi.mock("@/lib/clipboard", () => ({ writeToClipboard: vi.fn().mockResolvedValue(true) }));

type IdentityProps = ComponentProps<typeof SessionIdentity>;

const release: IdentityProps = {
  sessionId: "session-release",
  title: "Release notes",
  workspacePath: "/repo/flame",
};

function Host({ visible = true, ...identity }: IdentityProps & { visible?: boolean }) {
  return (
    <>
      <button>Outside</button>
      <div hidden={!visible}>
        <ContentVisibility visible={visible}>
          <SessionIdentity {...identity} />
        </ContentVisibility>
      </div>
    </>
  );
}

function moreButton(title: string) {
  return screen.getByRole("button", { name: `More actions for ${title}` });
}

type Route = "context" | "dropdown";

async function openMenu(route: Route, title: string) {
  if (route === "context") {
    fireEvent.contextMenu(screen.getByRole("heading", { level: 1, name: title }));
  } else {
    fireEvent.click(moreButton(title));
  }
  return screen.findByRole("menu", { name: title });
}

function itemLabels(menu: HTMLElement) {
  return within(menu)
    .getAllByRole("menuitem")
    .map((item) => item.textContent);
}

describe("SessionIdentity", () => {
  beforeEach(() => {
    vi.mocked(writeToClipboard).mockClear();
  });
  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  it("offers a native More actions button beside the unchanged title heading", () => {
    render(<Host {...release} />);

    const heading = screen.getByRole("heading", { level: 1, name: "Release notes" });
    const more = moreButton("Release notes");
    expect(more.tagName).toBe("BUTTON");
    expect(more.tabIndex).toBe(0);
    expect(more.getAttribute("aria-haspopup")).toBe("menu");
    expect(more.contains(heading)).toBe(false);
    expect(heading.closest("button")).toBeNull();
    expect(heading.getAttribute("title")).toBe("Release notes");
    expect(screen.getByText("flame").getAttribute("title")).toBe("/repo/flame");
  });

  it.each([
    {
      route: "context" as const,
      label: "Copy working directory",
      text: "/repo/flame",
      successLabel: "Working directory copied",
    },
    {
      route: "context" as const,
      label: "Copy session ID",
      text: "session-release",
      successLabel: "Session ID copied",
    },
    {
      route: "dropdown" as const,
      label: "Copy working directory",
      text: "/repo/flame",
      successLabel: "Working directory copied",
    },
    {
      route: "dropdown" as const,
      label: "Copy session ID",
      text: "session-release",
      successLabel: "Session ID copied",
    },
  ])("copies through the $route menu with $label", async ({ route, label, text, successLabel }) => {
    render(<Host {...release} />);

    const menu = await openMenu(route, "Release notes");
    expect(itemLabels(menu)).toEqual(["Copy working directory", "Copy session ID"]);
    fireEvent.click(within(menu).getByRole("menuitem", { name: label }));

    expect(writeToClipboard).toHaveBeenCalledExactlyOnceWith(text, { successLabel });
    await vi.waitFor(() => expect(menu.isConnected).toBe(false));
  });

  it.each(["context", "dropdown"] as const)(
    "offers only the session ID in the %s menu without a workspace",
    async (route) => {
      render(<Host sessionId="session-only" title="Draft" />);

      expect(screen.queryByText("/")).toBeNull();
      const menu = await openMenu(route, "Draft");
      expect(itemLabels(menu)).toEqual(["Copy session ID"]);
    },
  );

  it.each(["context", "dropdown"] as const)(
    "offers only the working directory in the %s menu without a session ID",
    async (route) => {
      render(<Host sessionId="" title="New session" workspacePath="/repo/flame" />);

      const menu = await openMenu(route, "New session");
      expect(itemLabels(menu)).toEqual(["Copy working directory"]);
    },
  );

  it("renders no menu trigger or empty menu when nothing can be copied", async () => {
    render(<Host sessionId="" title="New session" />);

    expect(screen.queryByRole("button", { name: /More actions/ })).toBeNull();
    fireEvent.contextMenu(screen.getByRole("heading", { level: 1, name: "New session" }));
    await act(async () => {
      await drainBrowserTasks();
    });
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it.each(["context", "dropdown"] as const)(
    "retires an open %s menu when the Session is replaced",
    async (route) => {
      const view = render(<Host {...release} />);
      const outside = screen.getByRole("button", { name: "Outside" });
      const menu = await openMenu(route, "Release notes");

      view.rerender(
        <Host sessionId="session-other" title="Other work" workspacePath="/repo/other" />,
      );
      act(() => outside.focus());

      await vi.waitFor(() => expect(menu.isConnected).toBe(false));
      expect(screen.queryByRole("menu")).toBeNull();
      expect(document.activeElement).toBe(outside);
      const next = await openMenu(route, "Other work");
      fireEvent.click(within(next).getByRole("menuitem", { name: "Copy session ID" }));
      expect(writeToClipboard).toHaveBeenCalledExactlyOnceWith("session-other", {
        successLabel: "Session ID copied",
      });
    },
  );

  it("keeps an open menu on a renamed Session with its current identity", async () => {
    const view = render(<Host {...release} />);
    const menu = await openMenu("dropdown", "Release notes");

    view.rerender(<Host {...release} title="Shipped notes" />);

    expect(menu.isConnected).toBe(true);
    expect(screen.getByRole("menu", { name: "Shipped notes" })).toBe(menu);
    expect(moreButton("Shipped notes")).toBeTruthy();
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Copy session ID" }));
    expect(writeToClipboard).toHaveBeenCalledExactlyOnceWith("session-release", {
      successLabel: "Session ID copied",
    });
  });

  it.each(["context", "dropdown"] as const)(
    "retires an open %s menu with its hidden host and keeps later outside focus",
    async (route) => {
      const view = render(<Host {...release} />);
      const outside = screen.getByRole("button", { name: "Outside" });
      const menu = await openMenu(route, "Release notes");

      act(() => {
        view.rerender(<Host {...release} visible={false} />);
        outside.focus();
      });
      await vi.waitFor(() => expect(menu.isConnected).toBe(false));
      await act(async () => {
        await drainBrowserTasks();
      });

      expect(document.activeElement).toBe(outside);
      view.rerender(<Host {...release} />);
      expect(screen.queryByRole("menu")).toBeNull();
      expect(writeToClipboard).not.toHaveBeenCalled();
    },
  );
});
