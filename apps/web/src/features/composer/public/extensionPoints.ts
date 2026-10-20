import {
  type ComposerKeyBindingSpec,
  type ComposerSubmitModeSpec,
  type SlashCommandSpec,
} from "@/features/composer/public/types";
import { defineExtensionPoint } from "@/extensions/contracts";
import { normalizeCombo } from "@/lib/combo";
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
  normalizeKey: normalizeCombo,
});

export const COMPOSER_SUBMIT_MODE = defineExtensionPoint<ComposerSubmitModeSpec>({
  id: "flame.composer.submitMode",
  keying: "single",
  keyOf: (mode) => mode.id,
});

export function useSlashCommands(): Array<{ cmd: string; spec: SlashCommandSpec }> {
  const entries = useExtensionEntries(SLASH_COMMAND);
  return useMemo(() => entries.map((entry) => ({ cmd: entry.key, spec: entry.item })), [entries]);
}

export function lookupSlashCommandOwner(cmd: string): string | undefined {
  return lookupExtensionOwner(SLASH_COMMAND, cmd);
}
