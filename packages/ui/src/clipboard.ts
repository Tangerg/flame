import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { useContentVisible } from "./content-visibility";

export async function copyText(text: string): Promise<boolean> {
  if (!text || typeof navigator === "undefined" || !navigator.clipboard)
    return false;
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

type CopyMaterial = (material: string) => Promise<boolean>;
interface CopySource {
  source: unknown;
  active: boolean;
  attempt: object | null;
}

export function useCopyAttempt(source: unknown): () => (() => boolean) | null {
  const visible = useContentVisible();
  const owner = useRef<CopySource>({ source, active: false, attempt: null });
  useLayoutEffect(() => {
    const binding: CopySource = { source, active: visible, attempt: null };
    owner.current = binding;
    return () => {
      binding.active = false;
      binding.attempt = null;
    };
  }, [source, visible]);
  return useCallback(() => {
    const binding = owner.current;
    if (!binding.active || binding.source !== source) return null;
    const attempt = {};
    binding.attempt = attempt;
    return () => binding.active && binding.attempt === attempt;
  }, [source]);
}

export function useCopyFeedback(
  material: string,
  resetAfterMs = 1500,
  copyMaterial: CopyMaterial = copyText,
): { copied: boolean; copy: () => Promise<boolean | undefined> } {
  const begin = useCopyAttempt(material);
  const visible = useContentVisible();
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [accepted, setAccepted] = useState<string | null>(null);
  const clear = useCallback(() => {
    clearTimeout(timer.current);
    timer.current = undefined;
  }, []);
  useLayoutEffect(() => {
    setAccepted(null);
    return clear;
  }, [material, visible, clear]);

  const copy = useCallback(async (): Promise<boolean | undefined> => {
    const current = begin();
    if (!current) return undefined;
    clear();
    setAccepted(null);
    const copied = await copyMaterial(material);
    if (!current()) return undefined;
    if (!copied) return false;
    setAccepted(material);
    timer.current = setTimeout(() => {
      timer.current = undefined;
      if (current()) setAccepted(null);
    }, resetAfterMs);
    return true;
  }, [begin, clear, copyMaterial, material, resetAfterMs]);
  return { copied: accepted === material, copy };
}
