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
