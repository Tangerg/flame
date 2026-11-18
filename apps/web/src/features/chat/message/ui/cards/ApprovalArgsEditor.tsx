import * as stylex from "@stylexjs/stylex";
import { useId, useLayoutEffect, useRef } from "react";
import { useT } from "@/lib/i18n/index";
import { SectionLabel } from "@/ui/index";
import { TextButton } from "@/ui/index";
import { Well } from "@/ui/index";
import { isImeKey, TextArea, vocab } from "@flame/ui";
import { face, space, type as typeStep } from "@flame/ui/tokens.stylex";
const ae = stylex.create({
  head: { marginBottom: space.s1, display: "flex", alignItems: "center", gap: space.s2 },
  label: { paddingInline: 0, paddingBlock: 0 },
  error: { marginTop: space.s1 },
});

export function ApprovalArgsEditor({
  editing,
  argsText,
  invalid,
  disabled,
  onEdit,
  onCancel,
  onTextChange,
}: {
  editing: boolean;
  argsText: string;
  invalid: boolean;
  disabled: boolean;
  onEdit: () => void;
  onCancel: () => void;
  onTextChange: (text: string) => void;
}) {
  const t = useT();
  const errorId = useId();
  const editor = useRef<HTMLTextAreaElement>(null);
  const editButton = useRef<HTMLButtonElement>(null);
  const cancelEdit = () => {
    onCancel();
    editButton.current?.focus({ preventScroll: true });
  };
  useLayoutEffect(() => {
    if (editing) editor.current?.focus();
  }, [editing]);
  return (
    <div
      onKeyDown={(event) => {
        if (
          !editing ||
          disabled ||
          event.key !== "Escape" ||
          event.defaultPrevented ||
          isImeKey(event.nativeEvent) ||
          !event.currentTarget.contains(event.target as Node)
        )
          return;
        event.preventDefault();
        event.stopPropagation();
        cancelEdit();
      }}
    >
      <div {...stylex.props(ae.head)}>
        <SectionLabel className={stylex.props(ae.label).className}>
          {t("approval.args.label")}
        </SectionLabel>
        <TextButton
          ref={editButton}
          type="button"
          shape="link"
          tone="accent"
          size="xs"
          disabled={disabled}
          focusableWhenDisabled
          onClick={editing ? cancelEdit : onEdit}
          className={stylex.props(vocab.strong).className}
        >
          {t(editing ? "common.cancel" : "approval.args.edit")}
        </TextButton>
      </div>
      {editing ? (
        <>
          <TextArea
            ref={editor}
            font="mono"
            invalid={invalid}
            value={argsText}
            readOnly={disabled}
            aria-disabled={disabled || undefined}
            aria-label={t("approval.args.label")}
            aria-describedby={invalid ? errorId : undefined}
            spellCheck={false}
            rows={Math.min(10, argsText.split("\n").length + 1)}
            onChange={(e) => {
              onTextChange(e.target.value);
            }}
            variant="well"
          />
          {invalid && (
            <div
              id={errorId}
              role="alert"
              {...stylex.props(ae.error, vocab.negative, typeStep.uiXs, face.mono)}
            >
              {t("approval.args.invalid")}
            </div>
          )}
        </>
      ) : (
        <Well cap="sm" ink="strong">
          {argsText}
        </Well>
      )}
    </div>
  );
}
