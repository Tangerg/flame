import type { ComponentProps } from "react";
import type { AgentActivityDisclosure, AgentRow } from "@/ui/agent/index";
import type { ReasoningBlock } from "@/features/chat/message/ui/cards/ReasoningBlock";
import type { ComposerImageAttachment } from "@/features/composer/public/attachments";
import type { DotTone } from "@/lib/tone";
import type { SystemMessage } from "@/ui/atoms/system-message";
import type { LabVariant } from "./story";
import { businessMessages as messages } from "./businessMessages";

export const statusTones = [
  "idle",
  "running",
  "waiting",
  "ok",
  "err",
] as const satisfies readonly DotTone[];

export const statusVariants = [
  { id: "all", title: messages.variants.all, longLabel: false },
  { id: "long", title: messages.variants.long, longLabel: true },
] as const;

type NavigationVariant = LabVariant & {
  readonly look: ComponentProps<typeof AgentRow>["look"];
  readonly detail: boolean;
  readonly longLabel: boolean;
  readonly disabled: boolean;
};

export const navigationVariants = [
  {
    id: "default",
    title: messages.variants.default,
    look: "row",
    detail: false,
    longLabel: false,
    disabled: false,
  },
  {
    id: "details",
    title: messages.variants.details,
    look: "row",
    detail: true,
    longLabel: false,
    disabled: false,
  },
  {
    id: "long",
    title: messages.variants.long,
    look: "row",
    detail: true,
    longLabel: true,
    disabled: false,
  },
  {
    id: "disabled",
    title: messages.variants.disabled,
    look: "row",
    detail: false,
    longLabel: false,
    disabled: true,
  },
] as const satisfies readonly NavigationVariant[];

type ActivityVariant = LabVariant & {
  readonly shell: ComponentProps<typeof AgentActivityDisclosure>["shell"];
  readonly tone: ComponentProps<typeof AgentActivityDisclosure>["tone"];
  readonly progress: boolean;
  readonly failed: boolean;
};

export const activityVariants = [
  {
    id: "line",
    title: messages.variants.line,
    shell: "line",
    tone: "neutral",
    progress: false,
    failed: false,
  },
  {
    id: "card",
    title: messages.variants.card,
    shell: "card",
    tone: "neutral",
    progress: false,
    failed: false,
  },
  {
    id: "progress",
    title: messages.variants.progress,
    shell: "card",
    tone: "neutral",
    progress: true,
    failed: false,
  },
  {
    id: "failure",
    title: messages.variants.failure,
    shell: "card",
    tone: "negative",
    progress: false,
    failed: true,
  },
] as const satisfies readonly ActivityVariant[];

export const referenceVariants = [
  { id: "short", title: messages.variants.short, long: false },
  { id: "long", title: messages.variants.long, long: true },
] as const satisfies readonly (LabVariant & { long: boolean })[];

export const reasoningVariants = [
  { id: "running", title: messages.variants.running, status: "running", short: false },
  { id: "complete", title: messages.variants.complete, status: "complete", short: false },
  { id: "incomplete", title: messages.variants.incomplete, status: "incomplete", short: false },
  { id: "short", title: messages.variants.short, status: "complete", short: true },
] as const satisfies readonly (LabVariant & {
  status: ComponentProps<typeof ReasoningBlock>["status"];
  short: boolean;
})[];

export const composerVariants = [
  {
    id: "default",
    title: messages.variants.default,
    disabled: false,
    compact: false,
    longLabel: false,
  },
  {
    id: "disabled",
    title: messages.variants.disabled,
    disabled: true,
    compact: false,
    longLabel: false,
  },
  {
    id: "compact",
    title: messages.variants.compact,
    disabled: false,
    compact: true,
    longLabel: false,
  },
  { id: "long", title: messages.variants.long, disabled: false, compact: false, longLabel: true },
] as const;

type AttachmentVariantId = Exclude<ComposerImageAttachment["status"], "ready"> | "reference";

export const attachmentVariants = [
  { id: "pending", title: messages.variants.pending },
  { id: "failed", title: messages.variants.failure },
  { id: "reference", title: messages.variants.reference },
] as const satisfies readonly (LabVariant & { id: AttachmentVariantId })[];

export const catalogVariants = [
  { id: "default", title: messages.variants.default, empty: false },
  { id: "empty", title: messages.variants.empty, empty: true },
] as const;

export const dataVariants = {
  ready: { id: "ready", title: messages.variants.ready },
  loading: { id: "loading", title: messages.variants.loading },
  empty: { id: "empty", title: messages.variants.empty },
  failure: { id: "failure", title: messages.variants.failure },
} as const;

type MessageTone = NonNullable<ComponentProps<typeof SystemMessage>["variant"]>;

export const messageVariants = [
  { id: "info", title: messages.variants.info },
  { id: "warning", title: messages.variants.warning },
  { id: "error", title: messages.variants.failure },
  { id: "success", title: messages.variants.success },
] as const satisfies readonly (LabVariant & { id: MessageTone })[];

export const fileVariants = [
  { id: "default", title: messages.variants.default, path: "apps/web/src/App.tsx" },
  {
    id: "long",
    title: messages.variants.long,
    path: "apps/web/src/features/workspace/application/a-very-long-recorded-workspace-file-for-layout-inspection.ts",
  },
  { id: "unicode", title: messages.variants.unicode, path: "工作区/界面实验室/组件示例.tsx" },
] as const;

export function businessVariant<T extends LabVariant>(variants: readonly T[], id: string): T {
  const selected = variants.find((variant) => variant.id === id);
  if (!selected) throw new Error(`Unknown business preview variant: ${id}`);
  return selected;
}
