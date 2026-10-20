import { Button, Metadata, RecordBody, RecordGroup, Toolbar } from "@flame/ui";
import { type Command, type Feedback } from "./types";
export function MemoryFeedback({
  feedback,
  original,
  pending,
  reconcile,
}: {
  feedback?: Feedback;
  original?: Command;
  pending: boolean;
  reconcile(): void;
}) {
  if (!feedback && !original) return null;
  return (
    <RecordGroup>
      <RecordGroup role={feedback?.state === "refused" ? "alert" : "status"}>
        <RecordBody>
          {feedback &&
            `${feedback.label} · ${
              feedback.state === "submitting"
                ? "submitting…"
                : feedback.state === "unknown"
                  ? "confirmation unknown. Check the original result before submitting another change."
                  : feedback.state === "refused"
                    ? "refused."
                    : "confirmed."
            }`}
        </RecordBody>
        {feedback?.detail && <RecordBody>{feedback.detail}</RecordBody>}
      </RecordGroup>
      {feedback?.state === "refused" && (
        <Metadata>Review the recorded result, then try again.</Metadata>
      )}
      {original && (
        <Toolbar label="Original Memory command">
          <Metadata>
            Original command: {original.promise.idempotencyKey}
          </Metadata>
          <Button disabled={pending} onClick={reconcile}>
            Check original result
          </Button>
        </Toolbar>
      )}
    </RecordGroup>
  );
}
