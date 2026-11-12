import { type RefObject, useRef } from "react";
import { Metadata, RecordBody, RecordGroup, Toolbar } from "@flame/ui";
import { failureText, type Translate } from "@flame/component-sdk";
import { MemoryAction } from "./MemoryAction";
import { type Command, type Feedback } from "./types";
export function MemoryFeedback({
  feedback,
  original,
  pending,
  reconcile,
  result: destination,
  t,
}: {
  feedback?: Feedback;
  original?: Command;
  pending: boolean;
  reconcile(): void;
  result?: RefObject<HTMLElement | null>;
  t: Translate;
}) {
  const own = useRef<HTMLElement>(null);
  const result = destination ?? own;
  if (!feedback && !original) return null;
  const detail =
    feedback?.failure &&
    failureText(
      feedback.failure.cause,
      t(`error.${feedback.failure.fallback}`),
    );
  const key = original && (
    <Metadata>
      {t("feedback.original", { key: original.promise.idempotencyKey })}
    </Metadata>
  );
  return (
    // A caller-owned destination sits outside any form, so this group is its reading unit.
    <RecordGroup data-focus-unit={destination ? "" : undefined}>
      <RecordGroup
        ref={result}
        tabIndex={-1}
        role={feedback?.state === "refused" ? "alert" : "status"}
      >
        <RecordBody>
          {feedback &&
            t(`feedback.${feedback.state}`, {
              label:
                feedback.intent.action === "add"
                  ? t(`command.add.${feedback.intent.scope}`)
                  : t(`command.${feedback.intent.action}`, {
                      id: feedback.intent.id,
                    }),
            })}
        </RecordBody>
        {detail && <RecordBody>{detail}</RecordBody>}
      </RecordGroup>
      {original?.source === "record" ? (
        <Toolbar label={t("feedback.original.label")}>
          {key}
          <MemoryAction result={result} pending={pending} onClick={reconcile}>
            {t("feedback.check")}
          </MemoryAction>
        </Toolbar>
      ) : (
        key
      )}
    </RecordGroup>
  );
}
