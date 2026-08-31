// Binds this pane's port to the two contexts that own these preferences: the chime belongs
// to the status context, the reveal mode to the transcript. The pane is the editor, not the
// owner — an adapter is exactly where that translation belongs.

import { useStreamRevealStore } from "@/plugins/builtin/chat/message/public/streamReveal";
import { useCompletionSoundStore } from "@/plugins/builtin/shell/status/public/completionSound";
import { configurePersonalizationPreferencesPort } from "../application/ports/preferences";

export function installPersonalizationPreferencesPort(): () => void {
  return configurePersonalizationPreferencesPort({
    useCompletionSound: () => useCompletionSoundStore((state) => state.completionSound),
    useSetCompletionSound: () => useCompletionSoundStore((state) => state.setCompletionSound),
    useStreamReveal: () => useStreamRevealStore((state) => state.streamReveal),
    useSetStreamReveal: () => useStreamRevealStore((state) => state.setStreamReveal),
  });
}
