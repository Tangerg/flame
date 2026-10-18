import { describe, expect, it } from "vitest";
import { DEFAULT_ACCENT } from "@/plugins/builtin/theme/public/appearance";
import {
  DEFAULT_ACCENTS,
  defaultMessageRoles,
  defaultStaticCommands,
} from "./defaultContributions";

describe("DEFAULT_ACCENTS", () => {
  it("keeps accent ids stable and ordered for the appearance picker", () => {
    expect(DEFAULT_ACCENTS.map((accent) => accent.id)).toEqual([
      "flame",
      "blue",
      "purple",
      "green",
    ]);
    expect(DEFAULT_ACCENTS.map((accent) => accent.order)).toEqual([0, 1, 2, 3]);
  });

  it("leads with the default accent so choosing it restores the default", () => {
    expect(DEFAULT_ACCENTS[0]!.color).toBe(DEFAULT_ACCENT);
  });

  it("names one distinct brand color for every built-in accent", () => {
    expect(new Set(DEFAULT_ACCENTS.map((accent) => accent.color)).size).toBe(
      DEFAULT_ACCENTS.length,
    );
  });
});

describe("defaultMessageRoles", () => {
  it("projects catalog keys into the three built-in message roles", () => {
    const roles = defaultMessageRoles();

    expect(roles).toEqual([
      {
        id: "user",
        displayName: "role.user",
        icon: "user",
        avatarVariant: "msg-user",
      },
      {
        id: "assistant",
        displayName: "role.assistant",
        icon: "spark",
        avatarVariant: "msg-agent",
      },
      {
        id: "system",
        displayName: "role.system",
        icon: "shield",
        avatarVariant: "msg-agent",
      },
    ]);
  });
});

describe("defaultStaticCommands", () => {
  it("projects the stable command ids, combos, and shortcut labels", () => {
    const run = () => {};
    const commands = defaultStaticCommands({
      toggleSidebar: run,
      toggleDock: run,
      toggleTheme: run,
      newChat: run,
      closeFocused: run,
      focusComposer: run,
      historyBack: run,
      historyForward: run,
      previousSession: run,
      nextSession: run,
    });

    expect(commands.map((command) => command.id)).toEqual([
      "view.toggle-sidebar",
      "view.toggle-dock",
      "settings.toggle-theme",
      "chat.new",
      "workspace.close-focused",
      "composer.focus",
      "history.back",
      "history.forward",
      "session.previous",
      "session.next",
    ]);
    expect(commands.map((command) => command.combo)).toEqual([
      "Mod+B",
      "Mod+Shift+B",
      "Mod+Shift+L",
      "Mod+N",
      "Mod+W",
      "Mod+L",
      "Mod+[",
      "Mod+]",
      "Mod+Shift+[",
      "Mod+Shift+]",
    ]);
    expect(commands.map((command) => command.label)).toEqual([
      "command.toggleSidebar",
      "command.toggleDock",
      "command.toggleTheme",
      "command.newChat",
      "command.closeFocused",
      "command.focusComposer",
      "command.historyBack",
      "command.historyForward",
      "command.previousSession",
      "command.nextSession",
    ]);
  });
});
