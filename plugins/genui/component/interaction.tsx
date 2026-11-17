import { useSyncExternalStore } from "react";
import { failureText, type Translate } from "@flame/component-sdk";
import {
  Button,
  CodeBlock,
  Metadata,
  ModalDialog,
  RecordGroup,
} from "@flame/ui";
import type { InteractionController } from "./interactionController";

export function InteractionFeedback({
  controller,
  t,
}: {
  controller: InteractionController;
  t: Translate;
}) {
  const { action: view } = useSyncExternalStore(
    controller.subscribe,
    controller.snapshot,
    controller.snapshot,
  );
  const feedback = (
    <>
      {view.status === "running" && (
        <p role="status" aria-live="polite">
          {t("interaction.running")}
        </p>
      )}
      {view.status === "failed" && (
        <p role="alert">{failureText(view.error, t("interaction.failed"))}</p>
      )}
      {view.status === "complete" && (
        <p role="status" aria-live="polite">
          {t("interaction.complete")}
        </p>
      )}
      {view.status === "unknown" && (
        <RecordGroup>
          <p role="alert">{t("interaction.unknown")}</p>
          {view.recoveries.map((recovery, index) => (
            <div key={recovery.key ?? index}>
              <Metadata>
                {recovery.label}
                {recovery.key ? ` · ${recovery.key}` : ""}
              </Metadata>
              {"recover" in recovery ? (
                <Button
                  onClick={() => {
                    void controller.recover(recovery);
                  }}
                >
                  {t("interaction.recover")}
                </Button>
              ) : (
                <p role="alert">
                  {failureText(recovery.error, t("interaction.failed"))}
                </p>
              )}
            </div>
          ))}
        </RecordGroup>
      )}
      {view.status === "review" && (
        <ModalDialog
          open
          title={t("interaction.review")}
          description={t("interaction.reviewDescription")}
          closeLabel={t("interaction.cancel")}
          onOpenChange={(open) => {
            if (!open) view.review.decide(false);
          }}
          footer={
            <>
              <Button onClick={() => view.review.decide(false)}>
                {t("interaction.cancel")}
              </Button>
              <Button
                variant="primary"
                onClick={() => view.review.decide(true)}
              >
                {t("interaction.confirm")}
              </Button>
            </>
          }
        >
          <dl>
            <dt>{t("interaction.installation")}</dt>
            <dd>{view.review.binding.installationId}</dd>
            <dt>{t("interaction.method")}</dt>
            <dd>{view.review.binding.method}</dd>
            <dt>{t("interaction.target")}</dt>
            <dd>
              {view.review.binding.target.scope}:{" "}
              {view.review.binding.target.reference}
            </dd>
          </dl>
          <CodeBlock
            language="json"
            label={t("interaction.input")}
            labels={{
              copy: t("code.copy"),
              wrap: t("code.wrap"),
              unwrap: t("code.unwrap"),
            }}
          >
            {view.review.input}
          </CodeBlock>
        </ModalDialog>
      )}
    </>
  );
  return feedback;
}
