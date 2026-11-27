import {
  type ComposerKeyBindingSpec,
  type SlashCommandSpec,
} from "@/features/composer/public/types";
import { defineExtensionPoint } from "@/extensions/contracts";
import { normalizeHotkey } from "@tanstack/hotkeys";
import { useMemo } from "react";
import { lookupExtensionOwner, useExtensionEntries } from "@/extensions/selectors/extensions";
export const SLASH_COMMAND = defineExtensionPoint<SlashCommandSpec>({
  id: "flame.composer.slashCommand",
  keying: "single",
  normalizeKey: (k) => (k.startsWith("/") ? k : `/${k}`),
});

export const COMPOSER_KEY_BINDING = defineExtensionPoint<ComposerKeyBindingSpec>({
  id: "flame.composer.keyBinding",
  keying: "single",
  keyOf: (s) => s.key,
  normalizeKey: normalizeHotkey,
});

export function useSlashCommandSuggestions() {
  const entries = useExtensionEntries(SLASH_COMMAND);
  return useMemo(
    () =>
      entries.flatMap(({ key, item: { description } }) =>
        description ? [{ command: key, description }] : [],
      ),
    [entries],
  );
}

export function lookupSlashCommandOwner(cmd: string): string | undefined {
  return lookupExtensionOwner(SLASH_COMMAND, cmd);
}
