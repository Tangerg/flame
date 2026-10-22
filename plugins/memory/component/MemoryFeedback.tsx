import { Button, Metadata, RecordBody, RecordGroup, Toolbar } from "@flame/ui";
import { type Translate } from "@flame/component-sdk";
import { type Command, type Feedback } from "./types";
export function MemoryFeedback({
  feedback,
  original,
  pending,
  reconcile,
  t,
}: {
  feedback?: Feedback;
  original?: Command;
  pending: boolean;
  reconcile(): void;
  t: Translate;
}) {
  if (!feedback && !original) return null;
  return (
    <RecordGroup>
      <RecordGroup role={feedback?.state === "refused" ? "alert" : "status"}>
        <RecordBody>
          {feedback &&
            t(`feedback.${feedback.state}`, { label: feedback.label })}
        </RecordBody>
        {feedback?.detail && <RecordBody>{feedback.detail}</RecordBody>}
      </RecordGroup>
      {feedback?.state === "refused" && (
        <Metadata>{t("feedback.refusedHint")}</Metadata>
      )}
      {original && (
        <Toolbar label={t("feedback.original.label")}>
          <Metadata>
            {t("feedback.original", {
              key: original.promise.idempotencyKey,
            })}
          </Metadata>
          <Button disabled={pending} onClick={reconcile}>
            {t("feedback.check")}
          </Button>
        </Toolbar>
      )}
    </RecordGroup>
  );
}
