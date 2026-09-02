import { useT } from "@/lib/i18n";
import { SectionLabel, TextArea, TextButton, Well } from "@/ui";

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
      <div className="mb-1 flex items-center gap-2">
        <SectionLabel className="px-0 py-0">{t("approval.args.label")}</SectionLabel>
        {!editing && (
          <TextButton
            type="button"
            tone="accent"
            size="sm"
            onClick={() => onEditToggle(true)}
            className="font-mono text-ui-xs font-semibold text-accent hover:underline"
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
            <div className="mt-1 font-mono text-ui-xs text-negative">
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
