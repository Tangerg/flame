import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { copyText } from "./clipboard";

type CopyMaterial = (material: string) => Promise<boolean>;

interface CopyFeedbackOwnership {
  material: string;
  lease: object;
  mounted: boolean;
  resetTimer: ReturnType<typeof setTimeout> | undefined;
}

function clearResetTimer(owner: CopyFeedbackOwnership): void {
  if (owner.resetTimer === undefined) return;
  clearTimeout(owner.resetTimer);
  owner.resetTimer = undefined;
}

/** Inline clipboard feedback owned by one exact piece of visible material.
 *
 * Streaming output, code and run digests can all replace their text without
 * replacing the button component. Each copy intent therefore carries both the
 * material it copied and a monotonic revision. A retired or older clipboard
 * response may have changed the system clipboard, but it cannot publish
 * "copied" into the material that replaced it or extend a newer intent's timer.
 */
export function useCopyFeedback(
  material: string,
  resetAfterMs = 1500,
  copyMaterial: CopyMaterial = copyText,
): { copied: boolean; copy: () => Promise<boolean> } {
  const ownerRef = useRef<CopyFeedbackOwnership>({
    material,
    lease: {},
    mounted: true,
    resetTimer: undefined,
  });
  const [accepted, setAccepted] = useState<{ material: string; lease: object } | null>(null);

  // Layout ownership changes before the replacement material can paint or
  // receive an event. Promise continuations run only after this transition has
  // retired the previous revision.
  useLayoutEffect(() => {
    const owner = ownerRef.current;
    if (owner.material === material) return;
    owner.material = material;
    owner.lease = {};
    clearResetTimer(owner);
    setAccepted(null);
  }, [material]);

  useLayoutEffect(() => {
    const owner = ownerRef.current;
    owner.mounted = true;
    return () => {
      owner.mounted = false;
      owner.lease = {};
      clearResetTimer(owner);
    };
  }, []);

  const copy = useCallback(async (): Promise<boolean> => {
    const owner = ownerRef.current;
    const lease = (owner.lease = {});
    clearResetTimer(owner);
    setAccepted(null);

    const accepted = await copyMaterial(material);
    if (!accepted || !owner.mounted || owner.material !== material || owner.lease !== lease) {
      return false;
    }

    setAccepted({ material, lease });
    owner.resetTimer = setTimeout(() => {
      owner.resetTimer = undefined;
      if (!owner.mounted || owner.material !== material || owner.lease !== lease) return;
      setAccepted(null);
    }, resetAfterMs);
    return true;
  }, [copyMaterial, material, resetAfterMs]);

  return {
    copied: accepted?.material === material,
    copy,
  };
}
