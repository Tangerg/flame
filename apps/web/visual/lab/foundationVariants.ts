import type { ButtonProps, Tone } from "@flame/ui";
import type { LabVariant } from "./story";

export const foundationViews = {
  styles: { id: "styles", title: { en: "Styles", "zh-CN": "样式对照" } },
  sizes: { id: "sizes", title: { en: "Sizes", "zh-CN": "尺寸对照" } },
  states: { id: "states", title: { en: "States", "zh-CN": "状态对照" } },
  editable: { id: "editable", title: { en: "Editable", "zh-CN": "可编辑" } },
  disabled: { id: "disabled", title: { en: "Disabled", "zh-CN": "禁用" } },
  pending: { id: "pending", title: { en: "Pending", "zh-CN": "处理中" } },
  invalid: { id: "invalid", title: { en: "Invalid", "zh-CN": "验证错误" } },
  autosize: { id: "autosize", title: { en: "Autosize", "zh-CN": "自动高度" } },
  continuous: { id: "continuous", title: { en: "Continuous", "zh-CN": "连续值" } },
  stepped: { id: "stepped", title: { en: "Stepped", "zh-CN": "步进值" } },
  compact: { id: "compact", title: { en: "Compact", "zh-CN": "紧凑" } },
  comfortable: { id: "comfortable", title: { en: "Comfortable", "zh-CN": "舒展" } },
  top: { id: "top", title: { en: "Top", "zh-CN": "上方" } },
  right: { id: "right", title: { en: "Right", "zh-CN": "右侧" } },
  bottom: { id: "bottom", title: { en: "Bottom", "zh-CN": "下方" } },
  left: { id: "left", title: { en: "Left", "zh-CN": "左侧" } },
  sm: { id: "sm", title: { en: "Small", "zh-CN": "小号" } },
  md: { id: "md", title: { en: "Medium", "zh-CN": "中号" } },
  lg: { id: "lg", title: { en: "Large", "zh-CN": "大号" } },
  confirmation: { id: "confirmation", title: { en: "Confirmation", "zh-CN": "操作确认" } },
  plain: { id: "plain", title: { en: "Plain", "zh-CN": "普通" } },
  row: { id: "row", title: { en: "Row", "zh-CN": "列表行" } },
  retained: { id: "retained", title: { en: "Retain draft", "zh-CN": "保留草稿" } },
  standalone: { id: "standalone", title: { en: "Standalone", "zh-CN": "独立面板" } },
  page: { id: "page", title: { en: "Hosted page", "zh-CN": "托管页面" } },
  list: { id: "list", title: { en: "List", "zh-CN": "列表" } },
  cards: { id: "cards", title: { en: "Cards", "zh-CN": "卡片" } },
  short: { id: "short", title: { en: "Short source", "zh-CN": "短源码" } },
  long: { id: "long", title: { en: "Long source", "zh-CN": "长源码" } },
  note: { id: "note", title: { en: "Note", "zh-CN": "备注" } },
  tip: { id: "tip", title: { en: "Tip", "zh-CN": "提示" } },
  important: { id: "important", title: { en: "Important", "zh-CN": "重要" } },
  warning: { id: "warning", title: { en: "Warning", "zh-CN": "警告" } },
  caution: { id: "caution", title: { en: "Caution", "zh-CN": "注意" } },
  unified: { id: "unified", title: { en: "Unified", "zh-CN": "合并视图" } },
  split: { id: "split", title: { en: "Split", "zh-CN": "并排视图" } },
  thumbnail: { id: "thumbnail", title: { en: "Thumbnail and lightbox", "zh-CN": "缩略图与灯箱" } },
  viewport: { id: "viewport", title: { en: "Image viewport", "zh-CN": "图片视口" } },
} as const satisfies Record<string, LabVariant>;

export const foundationButtonVariants = [
  "ghost",
  "soft",
  "outline",
  "primary",
  "wash",
  "tonal",
  "media",
  "mediaTray",
  "raised",
  "bare",
  "link",
] as const satisfies readonly NonNullable<ButtonProps["variant"]>[];

export const foundationControlSizes = [
  "xs",
  "sm",
  "md",
  "lg",
] as const satisfies readonly NonNullable<ButtonProps["size"]>[];

export const foundationTones = [
  "neutral",
  "accent",
  "success",
  "warning",
  "negative",
  "info",
] as const satisfies readonly Tone[];

export function foundationPlacement(value: string): "top" | "right" | "bottom" | "left" {
  const { top, right, bottom, left } = foundationViews;
  if (value === top.id || value === right.id || value === bottom.id || value === left.id)
    return value;
  throw new Error(`unknown foundation placement: ${value}`);
}
