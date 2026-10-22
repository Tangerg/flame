import { useMemo, useRef } from "react";
import {
  composerCompositionKeyIntent,
  type ComposerCompositionKeyIntent,
} from "@/features/composer/application/composerInputEvents";

// A freshly committed IME string lingers only briefly: an Enter within this
// window finishes composition and must not also submit. Past it, Enter submits.
const COMPOSITION_COMMIT_GRACE_MS = 100;

export interface CompositionCommitGuard {
  isComposing: () => boolean;
  start: () => void;
  end: () => void;
  noteChange: (nativeComposing: boolean) => void;
  clear: () => void;
  reset: () => void;
  keyIntent: (
    event: Pick<
      KeyboardEvent,
      "altKey" | "ctrlKey" | "isComposing" | "key" | "keyCode" | "metaKey" | "shiftKey"
    >,
  ) => ComposerCompositionKeyIntent;
}

export function useCompositionCommitGuard(): CompositionCommitGuard {
  const composingRef = useRef(false);
  const committedAtRef = useRef<number | null>(null);
  return useMemo(() => {
    const commitIsFresh = () =>
      committedAtRef.current !== null &&
      performance.now() - committedAtRef.current <= COMPOSITION_COMMIT_GRACE_MS;
    return {
      isComposing: () => composingRef.current,
      start: () => {
        composingRef.current = true;
        committedAtRef.current = null;
      },
      end: () => {
        composingRef.current = false;
        committedAtRef.current = performance.now();
      },
      noteChange: (nativeComposing) => {
        if (composingRef.current && !nativeComposing) {
          composingRef.current = false;
          committedAtRef.current = performance.now();
        }
      },
      clear: () => {
        committedAtRef.current = null;
      },
      reset: () => {
        composingRef.current = false;
        committedAtRef.current = null;
      },
      keyIntent: (event) => {
        const intent = composerCompositionKeyIntent(event, composingRef.current, commitIsFresh());
        committedAtRef.current = null;
        return intent;
      },
    };
  }, []);
}
