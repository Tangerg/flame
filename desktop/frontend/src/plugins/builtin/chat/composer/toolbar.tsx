import { useRef } from "react";

import { DropdownMenu, HiddenFileInput, Icon, IconButton } from "@/ui";
import { AgentComposerChip } from "@/ui/agent";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import { useSelectedModel, useSelectedModelSelection } from "./public/selectedModel";
import {
  APPROVAL_MODES,
  DEFAULT_APPROVAL_MODE,
  setApprovalMode,
  useApprovalMode,
  type ApprovalMode,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { rpcErrorText } from "@/lib/rpcErrors";
import { contributeLayout, notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { definePlugin } from "@/plugins/sdk";
import { useAddComposerImageFiles } from "./public/attachments";
import { useSetComposerModelPreference } from "./public/modelPreference";
import { ModelPicker } from "./ui/ModelPicker";

function ReasoningEffortPicker() {
  const t = useT();
  const selection = useSelectedModelSelection();
  const setModel = useSetComposerModelPreference();
  if (!selection || selection.model.reasoningLevels.length === 0) return null;

  const { model, reasoningEffort } = selection;
  const selectedEffort = reasoningEffort ?? model.reasoningLevelOrDefault();
  if (!selectedEffort) return null;

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <AgentComposerChip
            aria-label={t("composer.switchReasoningEffort")}
            className="capitalize"
            leading={<Icon name="sparkle" size="sm" className="text-fg-faint" />}
            label={selectedEffort}
          />
        }
      />
      <DropdownMenu.Content align="start" sideOffset={6} className="min-w-[176px]">
        {model.reasoningLevels.map((effort) => (
          <DropdownMenu.Item
            key={effort}
            onClick={() =>
              setModel({
                kind: "explicit",
                provider: model.provider,
                model: model.id,
                reasoningEffort: effort,
              })
            }
            className="grid grid-cols-[minmax(0,1fr)_14px] items-center gap-2 px-2"
          >
            <span className="truncate capitalize">{effort}</span>
            {effort === selectedEffort && <Icon name="check" size="xs" className="text-accent" />}
          </DropdownMenu.Item>
        ))}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

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
        className="disabled:opacity-25"
      />
    </>
  );
}

function ApprovalModePill() {
  const t = useT();
  const { data: mode, isError } = useApprovalMode();
  if (isError || mode === undefined) return null;
  const current = APPROVAL_MODES.find((m) => m.value === mode) ?? DEFAULT_APPROVAL_MODE;
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
            className={cn(
              "font-medium",
              full ? "text-warning hover:bg-warning-wash" : "hover:bg-hover hover:text-fg",
            )}
            leading={<Icon name={full ? "alert" : "shield"} size="sm" className="opacity-100" />}
            label={t(current.labelKey)}
          />
        }
      />
      <DropdownMenu.Content align="start" sideOffset={6} className="min-w-[248px]">
        {APPROVAL_MODES.map((m) => (
          <DropdownMenu.Item
            key={m.value}
            onClick={() => void onSelect(m.value)}
            className="grid grid-cols-[minmax(0,1fr)_14px] items-start gap-2 rounded-md px-2 py-1.5 outline-none data-[highlighted]:bg-hover"
          >
            <span className="min-w-0">
              <span className="block text-ui-md font-semibold text-fg">{t(m.labelKey)}</span>
              <span className="block text-ui-sm leading-snug text-fg-muted">{t(m.descKey)}</span>
            </span>
            {m.value === mode && <Icon name="check" size="xs" className="mt-0.5 text-accent" />}
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
      order: 1,
      component: ApprovalModePill,
    });
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "model",
      order: 2,
      component: ModelPicker,
    });
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "reasoning-effort",
      order: 3,
      component: ReasoningEffortPicker,
    });
  },
});
