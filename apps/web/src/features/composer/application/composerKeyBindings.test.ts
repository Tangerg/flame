import { describe, expect, it } from "vitest";
import { composerKeyBindings } from "@/features/composer/application/composerKeyBindings";
describe("composer key bindings", () => {
  it("maps each semantic action to its stable key and description", () => {
    const handler = () => true;
    const bindings = composerKeyBindings({
      send: handler,
      declineApproval: handler,
      stopRun: handler,
      historyPrevious: handler,
      historyNext: handler,
    });

    expect(bindings.map(({ key, description }) => [key, description])).toEqual([
      ["Mod+Enter", "composer.key.sendDesc"],
      ["Mod+Shift+Backspace", "composer.key.declineDesc"],
      ["Escape", "composer.key.stopDesc"],
      ["Mod+ArrowUp", "composer.key.historyPrevDesc"],
      ["Mod+ArrowDown", "composer.key.historyNextDesc"],
    ]);
  });
});
