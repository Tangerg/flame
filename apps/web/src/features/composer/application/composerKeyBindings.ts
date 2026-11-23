import { type ComposerKeyBindingSpec } from "@/features/composer/public/types";
type ComposerKeyHandler = ComposerKeyBindingSpec["handler"];

interface ComposerKeyHandlers {
  send: ComposerKeyHandler;
  declineApproval: ComposerKeyHandler;
  stopRun: ComposerKeyHandler;
  historyPrevious: ComposerKeyHandler;
  historyNext: ComposerKeyHandler;
}

export function composerKeyBindings(handlers: ComposerKeyHandlers): ComposerKeyBindingSpec[] {
  return [
    {
      key: "Mod+Enter",
      description: "composer.key.sendDesc",
      handler: handlers.send,
    },
    {
      key: "Mod+Shift+Backspace",
      description: "composer.key.declineDesc",
      handler: handlers.declineApproval,
    },
    { key: "Escape", description: "composer.key.stopDesc", handler: handlers.stopRun },
    {
      key: "Mod+ArrowUp",
      description: "composer.key.historyPrevDesc",
      handler: handlers.historyPrevious,
    },
    {
      key: "Mod+ArrowDown",
      description: "composer.key.historyNextDesc",
      handler: handlers.historyNext,
    },
  ];
}
