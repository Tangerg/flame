import { useEffectEvent, useLayoutEffect, useState } from "react";
import { dropTargetForExternal } from "@atlaskit/pragmatic-drag-and-drop/adapter/drop-target-for-external";
import { monitorForExternal } from "@atlaskit/pragmatic-drag-and-drop/adapter/monitor-for-external";
import { type ExternalDragPayload } from "@atlaskit/pragmatic-drag-and-drop/adapter/external-adapter-types";
import { containsFiles } from "@atlaskit/pragmatic-drag-and-drop/utils/contains-files";
import { getFiles } from "@atlaskit/pragmatic-drag-and-drop/utils/get-files";
import { preventUnhandled } from "@atlaskit/pragmatic-drag-and-drop/utils/prevent-unhandled";
import { imageFiles } from "@/features/composer/public/input";
import { type ComposerImageIntake } from "@/features/composer/application/state";
import { useComposer } from "@/features/composer/public/composer";
import { useContentVisible, Icon } from "@/ui/index";
import * as stylex from "@stylexjs/stylex";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { composerStyles } from "@/features/composer/ui/composerStyles";
import { useT } from "@/lib/i18n/index";

export function useComposerFileDrop(
  acceptsImages: boolean,
  onFiles: (files: File[], intake: ComposerImageIntake) => void,
) {
  const intake = useComposer().useImageIntake();
  const visible = useContentVisible();
  const deliver = useEffectEvent(onFiles);
  const [element, bindTarget] = useState<HTMLElement | null>(null);
  const [dragging, setDragging] = useState(false);
  useLayoutEffect(() => {
    if (!element || !visible || !intake.current()) return;
    let phase: "registering" | "listening" | "retired" = "registering";
    let observed: ExternalDragPayload | null = null;
    const current = () =>
      phase !== "retired" &&
      element.isConnected &&
      !element.closest("[hidden], [inert]") &&
      intake.current();
    const stopMonitor = monitorForExternal({
      canMonitor(args) {
        if (!current() || !containsFiles(args)) return false;
        // PDD also evaluates new monitors during an active drag; that observation grants no files.
        if (phase === "listening") observed = args.source;
        preventUnhandled.start();
        return true;
      },
      onDragStart({ source }) {
        if (!current() || observed !== source) return;
        setDragging(true);
      },
      onDrop() {
        if (phase === "retired") return;
        setDragging(false);
        preventUnhandled.stop();
      },
    });
    phase = "listening";
    const stopTarget = dropTargetForExternal({
      element,
      canDrop: (args) => current() && observed === args.source && containsFiles(args),
      onDrop({ source, location }) {
        if (!current() || !observed || location.current.dropTargets[0]?.element !== element) return;
        const files = imageFiles(getFiles({ source }));
        if (files.length) deliver(files, intake);
      },
    });
    return () => {
      phase = "retired";
      setDragging(false);
      stopTarget();
      stopMonitor();
      preventUnhandled.stop();
    };
  }, [element, visible, intake, acceptsImages]);
  return { ref: bindTarget, dragging };
}

export function ComposerFileDropCue({ acceptsImages }: { acceptsImages: boolean }) {
  const t = useT();
  return (
    <div
      data-slot="composer-drop-cue"
      aria-live="polite"
      {...stylex.props(composerStyles.dropCue, !acceptsImages && composerStyles.dropCueRefused)}
    >
      <Icon name={acceptsImages ? "image" : "alert"} size="md" />
      <span {...stylex.props(composerStyles.dropLabel, typeStep.uiMd)}>
        {acceptsImages ? t("composer.drop.files") : t("composer.attachImage.unsupported")}
      </span>
    </div>
  );
}
