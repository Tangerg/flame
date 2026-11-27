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
  const view = useSyncExternalStore(
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
          {view.recoveries.map(({ key, label }) => (
            <div key={key}>
              <Metadata>
                {label} · {key}
              </Metadata>
              <Button onClick={() => void controller.recover(key)}>
                {t("interaction.recover")}
              </Button>
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
            if (!open) view.review.cancel();
          }}
          footer={
            <>
              <Button onClick={() => view.review.cancel()}>
                {t("interaction.cancel")}
              </Button>
              <Button variant="primary" onClick={() => view.review.approve()}>
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
