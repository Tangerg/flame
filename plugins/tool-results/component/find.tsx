import type { CSSProperties, KeyboardEvent, ReactNode, RefObject } from "react";
import { X } from "@flame/icons/react";
import { IconButton, isImeKey, SearchField } from "@flame/ui";
import type { Translate } from "@flame/component-sdk";
import { frameInset } from "./frame";

export const findBar: CSSProperties = {
  display: "flex",
  minWidth: 0,
  paddingInline: `${frameInset} calc(var(--spacing) * 1.5)`,
  paddingBlockEnd: "var(--layout-label-gap)",
  fontFamily: "var(--font-sans)",
  fontSize: "var(--fs-ui-sm)",
  lineHeight: "var(--leading-body)",
};

export function isFindShortcut(event: KeyboardEvent) {
  return (
    !event.defaultPrevented &&
    !isImeKey(event.nativeEvent) &&
    (event.metaKey || event.ctrlKey) &&
    event.key.toLowerCase() === "f"
  );
}

export function containsQuery(text: string, query: string) {
  return text.toLocaleLowerCase().includes(query.toLocaleLowerCase());
}

// Repeating Find on the already-focused field fires no focus-in, so every
// explicit entry also asks the containing scroller to reveal the field.
export function enterFind(field: HTMLElement | null) {
  if (!field) return;
  field.focus({ preventScroll: true });
  field.scrollIntoView({
    block: "nearest",
    inline: "nearest",
    behavior: "instant",
  });
}

export function FindField({
  t,
  label,
  fieldLabel,
  value,
  fieldRef,
  status,
  onChange,
  onClose,
  onEnter,
}: {
  t: Translate;
  label: string;
  fieldLabel: string;
  value: string;
  fieldRef: RefObject<HTMLInputElement | null>;
  status?: ReactNode;
  onChange: (value: string) => void;
  onClose: () => void;
  onEnter?: (backward: boolean) => void;
}) {
  return (
    // Find is a bounded task: Escape closes it even over a query, so the
    // close runs in capture before the field's own clear-on-Escape.
    <div
      role="search"
      aria-label={label}
      onKeyDownCapture={(event) => {
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          isImeKey(event.nativeEvent)
        )
          return;
        event.preventDefault();
        onClose();
      }}
      onKeyDown={(event) => {
        if (!isFindShortcut(event)) return;
        event.preventDefault();
        event.stopPropagation();
        enterFind(fieldRef.current);
      }}
      style={{
        display: "flex",
        flex: "1 1 14rem",
        alignItems: "center",
        gap: "var(--layout-control-gap)",
        minWidth: 0,
      }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        <SearchField
          ref={fieldRef}
          size="sm"
          aria-label={fieldLabel}
          placeholder={fieldLabel}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onClear={() => onChange("")}
          clearLabel={t("find.clear")}
          onKeyDown={(event) => {
            if (
              event.key === "Enter" &&
              onEnter &&
              !isImeKey(event.nativeEvent)
            ) {
              event.preventDefault();
              onEnter(event.shiftKey);
            }
          }}
        />
      </div>
      {status}
      <IconButton
        icon={X}
        size="xs"
        quiet
        aria-label={t("find.close")}
        onClick={onClose}
      />
    </div>
  );
}
