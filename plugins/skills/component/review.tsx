import { type Translate } from "@flame/component-sdk";
import {
  Button,
  RecordForm,
  FormSection,
  FormActions,
  Disclosure,
  Facts,
  RecordBody,
  RecordTitle,
} from "@flame/ui";
import { type SkillsState } from "./useSkills";

const reviewPresentation = {
  approve: { variant: "primary" },
  reject: { variant: "tonal", tone: "negative" },
} as const;

export function SkillReviewForm({
  t,
  ref,
  review,
  workspacePath,
  busy,
  disabled,
  onCancel,
  onConfirm,
}: {
  t: Translate;
  ref: SkillsState["reviewElement"];
  review: NonNullable<SkillsState["review"]>;
  workspacePath: string;
  busy: boolean;
  disabled: boolean;
  onCancel(): void;
  onConfirm(): void;
}) {
  return (
    <RecordForm ref={ref} aria-label={t("review.label")} data-focus-unit="">
      <RecordTitle>
        {t(`review.title.${review.decision}`, {
          name: review.proposal.name,
        })}
      </RecordTitle>
      <FormSection title={t("review.proposed")}>
        <RecordBody>{review.proposal.description}</RecordBody>
        <RecordBody>{review.proposal.instructions}</RecordBody>
      </FormSection>
      <Disclosure title={t("review.target")}>
        <Facts
          values={[
            [t("fact.workspace"), workspacePath],
            [t("fact.scope"), t(`scope.${review.proposal.scope}`)],
            [t("fact.revision"), review.proposal.revision],
          ]}
        />
      </Disclosure>
      <FormActions label={t("review.confirmation")}>
        <Button variant="ghost" disabled={busy} autoFocus onClick={onCancel}>
          {t("cancel")}
        </Button>
        <Button
          {...reviewPresentation[review.decision]}
          disabled={disabled}
          onClick={onConfirm}
        >
          {t(`confirm.${review.decision}`)}
        </Button>
      </FormActions>
    </RecordForm>
  );
}
