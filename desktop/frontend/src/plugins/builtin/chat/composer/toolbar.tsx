import { useRef } from "react";

import * as stylex from "@stylexjs/stylex";
import { DropdownMenu, HiddenFileInput, Icon, IconButton, vocab } from "@/ui";
import type { IconName } from "@/ui/icons";
import { type as typeStep } from "@/styles/tokens.stylex";
import { toolbarStyles } from "./toolbarStyles";
import { AgentComposerChip } from "@/ui/agent";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import { useSelectedModel } from "./public/selectedModel";
import {
  APPROVAL_MODE_OPTION,
  APPROVAL_MODES,
  setApprovalMode,
  useApprovalMode,
  type ApprovalMode,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { rpcErrorText } from "@/lib/rpcErrors";
import { contributeLayout, notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { useAddComposerImageFiles } from "./public/attachments";
import { ModelPicker } from "./ui/ModelPicker";

function AttachButton() {
  const t = useT();
  const addImageFiles = useAddComposerImageFiles();
  const inputRef = useRef<HTMLInputElement>(null);
  const canAttach = useSelectedModel()?.acceptsInput("image") ?? false;

  return (
    <>
      <HiddenFileInput
        ref={inputRef}
        accept="image/*"
        multiple
        aria-label={t("composer.attachImage")}
        onChange={(e) => {
          const files = imageFiles(e.target.files);
          e.target.value = "";
          if (files.length > 0) addImageFiles(files);
        }}
      />
      <IconButton
        icon="plus"
        aria-label={t("composer.attachImage")}
        title={canAttach ? t("composer.attachImage") : t("composer.attachImage.unsupported")}
        disabled={!canAttach}
        onClick={() => inputRef.current?.click()}
      />
    </>
  );
}

const MODE_ICON: Record<ApprovalMode, IconName> = {
  safe: "shield",
  balanced: "gauge",
  yolo: "alert",
};

function ApprovalModePill() {
  const t = useT();
  const { data: mode, isError } = useApprovalMode();
  if (isError || mode === undefined) return null;
  const current = APPROVAL_MODE_OPTION[mode];
  const full = mode === "yolo";
  const onSelect = async (next: ApprovalMode) => {
    if (next === mode) return;
    try {
      await setApprovalMode(next);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("approvals.error.mode"));
    }
  };
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <AgentComposerChip
            type="button"
            aria-label={t("approvals.mode.aria")}
            variant={full ? "wash" : "ghost"}
            tone={full ? "warning" : undefined}
            leading={<Icon name={MODE_ICON[mode]} size="sm" full />}
            label={t(current.labelKey)}
            labelVisibility={full ? "always" : "wide"}
          />
        }
      />
      <DropdownMenu.Content align="start" sideOffset={6}>
        <div aria-hidden {...stylex.props(toolbarStyles.menuHeading, typeStep.uiSm)}>
          {t("approvals.mode.aria")}
        </div>
        {APPROVAL_MODES.map((m) => (
          <DropdownMenu.Item
            key={m.value}
            onClick={() => void onSelect(m.value)}
            layout="pick"
            styles={toolbarStyles.describedRow}
          >
            <Icon
              name={MODE_ICON[m.value]}
              size="md"
              className={stylex.props(toolbarStyles.optionGlyph).className}
            />
            <span {...stylex.props(vocab.min)}>
              <span {...stylex.props(toolbarStyles.optionTitle, typeStep.uiMd)}>
                {t(m.labelKey)}
              </span>
              <span {...stylex.props(toolbarStyles.optionDetail, typeStep.uiSm)}>
                {t(m.descKey)}
              </span>
            </span>
            {m.value === mode && (
              <Icon
                name="check"
                size="xs"
                className={stylex.props(toolbarStyles.checkTop, vocab.accent).className}
              />
            )}
          </DropdownMenu.Item>
        ))}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

export const composerToolbar = definePlugin({
  name: "flame.builtin.composer-toolbar",
  setup(ctx) {
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "attach",
      order: 0,
      component: AttachButton,
    });
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "approval",
      order: 3,
      component: ApprovalModePill,
    });
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "model",
      order: 1,
      component: ModelPicker,
    });
  },
});
