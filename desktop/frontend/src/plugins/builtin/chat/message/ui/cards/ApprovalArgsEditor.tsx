import * as stylex from "@stylexjs/stylex";
import { useT } from "@/lib/i18n";
import { SectionLabel, TextArea, TextButton, vocab, Well } from "@/ui";
import { face, space, type as typeStep } from "@/styles/tokens.stylex";

const ae = stylex.create({
  head: { marginBottom: space.s1, display: "flex", alignItems: "center", gap: space.s2 },
  label: { paddingInline: 0, paddingBlock: 0 },
  error: { marginTop: space.s1 },
});

export function ApprovalArgsEditor({
  editing,
  argsText,
  invalid,
  onEditToggle,
  onTextChange,
}: {
  editing: boolean;
  argsText: string;
  invalid: boolean;
  onEditToggle: (editing: boolean) => void;
  onTextChange: (text: string) => void;
}) {
  const t = useT();
  return (
    <div>
      <div {...stylex.props(ae.head)}>
        <SectionLabel className={stylex.props(ae.label).className}>
          {t("approval.args.label")}
        </SectionLabel>
        {!editing && (
          <TextButton
            type="button"
            shape="link"
            tone="accent"
            size="xs"
            onClick={() => onEditToggle(true)}
            className={stylex.props(vocab.strong).className}
          >
            {t("approval.args.edit")}
          </TextButton>
        )}
      </div>
      {editing ? (
        <>
          <TextArea
            invalid={invalid}
            value={argsText}
            aria-label={t("approval.args.label")}
            spellCheck={false}
            rows={Math.min(10, argsText.split("\n").length + 1)}
            onChange={(e) => {
              onTextChange(e.target.value);
            }}
            variant="well"
          />
          {invalid && (
            <div {...stylex.props(ae.error, vocab.negative, typeStep.uiXs, face.mono)}>
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
