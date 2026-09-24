import { useEffect, useEffectEvent, useRef, useState } from "react";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import * as stylex from "@stylexjs/stylex";
import { Icon } from "@/ui";
import { type as typeStep } from "@/styles/tokens.stylex";
import { composerStyles } from "./composerStyles";
import { useT } from "@/lib/i18n";
import { hasComposerImageTransferItems } from "../application/composerInputEvents";

export function useComposerImageDrop(onDropImages: (files: File[]) => void): boolean {
  return useWindowImageDrag(onDropImages);
}

function useWindowImageDrag(onDropImages: (files: File[]) => void): boolean {
  const [dragging, setDragging] = useState(false);
  const depth = useRef(0);
  const deliverImages = useEffectEvent(onDropImages);

  useEffect(() => {
    const hasImageItems = (dt: DataTransfer | null | undefined): boolean =>
      hasComposerImageTransferItems(dt?.items);

    const onEnter = (event: DragEvent): void => {
      depth.current += 1;
      if (hasImageItems(event.dataTransfer)) setDragging(true);
    };
    const onLeave = (): void => {
      depth.current -= 1;
      if (depth.current <= 0) {
        depth.current = 0;
        setDragging(false);
      }
    };
    const onOver = (event: DragEvent): void => {
      if (hasImageItems(event.dataTransfer)) event.preventDefault();
    };
    const onDrop = (event: DragEvent): void => {
      depth.current = 0;
      setDragging(false);
      if (event.defaultPrevented) return;
      const files = imageFiles(event.dataTransfer?.files);
      if (files.length === 0) return;
      event.preventDefault();
      deliverImages(files);
    };

    window.addEventListener("dragenter", onEnter);
    window.addEventListener("dragleave", onLeave);
    window.addEventListener("dragover", onOver);
    window.addEventListener("drop", onDrop);
    return () => {
      window.removeEventListener("dragenter", onEnter);
      window.removeEventListener("dragleave", onLeave);
      window.removeEventListener("dragover", onOver);
      window.removeEventListener("drop", onDrop);
      depth.current = 0;
    };
  }, []);

  return dragging;
}

export function ComposerDropCue({ acceptsImages }: { acceptsImages: boolean }) {
  const t = useT();
  return (
    <div
      data-slot="composer-drop-cue"
      aria-live="polite"
      {...stylex.props(composerStyles.dropCue, !acceptsImages && composerStyles.dropCueRefused)}
    >
      <Icon name={acceptsImages ? "image" : "alert"} size="md" />
      <span {...stylex.props(composerStyles.dropLabel, typeStep.uiMd)}>
        {acceptsImages ? t("composer.drop.images") : t("composer.attachImage.unsupported")}
      </span>
    </div>
  );
}
