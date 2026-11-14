import { useEffectEvent, useLayoutEffect, useMemo, useState, type RefObject } from "react";
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
  target: RefObject<HTMLElement | null>,
  acceptsImages: boolean,
  onFiles: (files: File[], intake: ComposerImageIntake) => void,
): boolean {
  const intake = useComposer().useImageIntake();
  const visible = useContentVisible();
  const deliver = useEffectEvent(onFiles);
  const registration = useMemo(
    () => ({ intake, visible, acceptsImages }),
    [intake, visible, acceptsImages],
  );
  const [dragging, setDragging] = useState<object | null>(null);
  useLayoutEffect(() => {
    const element = target.current;
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
        setDragging(registration);
      },
      onDrop() {
        if (phase === "retired") return;
        setDragging(null);
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
      stopTarget();
      stopMonitor();
      preventUnhandled.stop();
    };
  }, [target, visible, intake, acceptsImages, registration]);
  return dragging === registration;
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
