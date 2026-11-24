import * as stylex from "@stylexjs/stylex";
import { type RefObject } from "react";
import { useT } from "@/lib/i18n/index";
import { SystemMessage } from "@/ui/index";

const styles = stylex.create({
  feedback: { flexShrink: 0, maxHeight: "40dvh", overflowY: "auto", overscrollBehavior: "contain" },
});

export interface LocalFileActions {
  open: (cwd: string, path: string) => void;
  reveal: (cwd: string, path: string) => void;
  failure?: string;
  dismissFeedback(): void;
}

export function LocalFileFeedback({
  actions,
  focusTarget,
}: {
  actions: LocalFileActions;
  focusTarget: RefObject<HTMLElement | null>;
}) {
  const t = useT();
  if (actions.failure === undefined) return null;
  return (
    <div {...stylex.props(styles.feedback)}>
      <SystemMessage
        variant="error"
        data-slot="local-file-feedback"
        action={{
          label: t("common.close"),
          onClick: () => {
            const host = focusTarget.current;
            const active = host?.ownerDocument.activeElement;
            if (
              active instanceof HTMLElement &&
              active.closest('[data-slot="local-file-feedback"]')
            )
              host?.focus({ preventScroll: true });
            actions.dismissFeedback();
          },
        }}
      >
        {actions.failure}
      </SystemMessage>
    </div>
  );
}
