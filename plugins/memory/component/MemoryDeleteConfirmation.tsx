import { type ReactNode } from "react";
import { type Translate } from "@flame/component-sdk";
import {
  Button,
  FormActions,
  Metadata,
  RecordBody,
  RecordForm,
  RecordGroup,
  RecordMeta,
  RecordTitle,
} from "@flame/ui";
import { type MemoryItem } from "./types";

type DeleteConfirmationProps = {
  item: MemoryItem;
  opener: () => HTMLElement | null;
  feedback: ReactNode;
  pending: boolean;
  locked: boolean;
  recovering: boolean;
  aborted: boolean;
  cancel: () => void;
  confirm: () => void;
  t: Translate;
};

export function MemoryDeleteConfirmation({
  item,
  opener,
  feedback,
  pending,
  locked,
  recovering,
  aborted,
  cancel,
  confirm,
  t,
}: DeleteConfirmationProps) {
  return (
    <RecordForm
      aria-label={t("delete.confirm.label")}
      opener={opener}
      data-focus-unit=""
    >
      <RecordGroup>
        <RecordTitle>{t("delete.title")}</RecordTitle>
        <RecordMeta>
          <span>{t(`scope.${item.Scope}`)}</span>
          <span>{item.ID}</span>
        </RecordMeta>
        <RecordBody>{item.Content}</RecordBody>
        <Metadata>{t("delete.notice")}</Metadata>
      </RecordGroup>
      <FormActions label={t("delete.actions.label")} feedback={feedback}>
        <Button
          variant="ghost"
          disabled={locked}
          focusableWhenDisabled
          // oxlint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
          onClick={cancel}
        >
          {t("cancel")}
        </Button>
        <Button
          variant={recovering ? "primary" : "tonal"}
          tone={recovering ? undefined : "negative"}
          pending={pending}
          disabled={recovering ? aborted : locked}
          focusableWhenDisabled={locked}
          onClick={confirm}
        >
          {recovering ? t("feedback.check") : t("delete.confirm")}
        </Button>
      </FormActions>
    </RecordForm>
  );
}
