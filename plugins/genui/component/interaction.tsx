import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import {
  failureText,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import {
  Button,
  CodeBlock,
  Metadata,
  ModalDialog,
  RecordGroup,
} from "@flame/ui";
import { useGetFieldValue, useSetFieldValue } from "@openuidev/react-lang";
import {
  operationKey,
  Presentation,
  safeURL,
  stateKey,
} from "../contracts/document";
import { InteractionController } from "./interactionController";

interface Interaction {
  blocked: boolean;
  execute(actionId: string): Promise<void>;
  reset(fields?: string[]): void;
  resetForm(actionId: string): void;
  update(id: string, value: unknown): void;
  readInitial(id: string): unknown;
  feedback: ReactNode;
}

const InteractionContext = createContext<Interaction | null>(null);

export function useGenUIInteraction(): Interaction {
  const interaction = useContext(InteractionContext);
  if (!interaction)
    throw new Error("genui: interaction requires its document owner");
  return interaction;
}

export function GenUIInteractionProvider({
  context,
  presentation,
  t,
  children,
}: {
  context: ComponentContext;
  presentation: Presentation;
  t: Translate;
  children: ReactNode;
}) {
  const get = useGetFieldValue();
  const set = useSetFieldValue();
  const initial = useMemo(
    () =>
      new Map(
        presentation.snapshot().state.map(({ id, value }) => [id, value]),
      ),
    [presentation],
  );
  const controller = useMemo(
    () =>
      new InteractionController(
        context,
        presentation,
        {
          readState: (id) => {
            const value = get(undefined, stateKey(id));
            return value === undefined ? initial.get(id) : value;
          },
          writeState: (id, value) =>
            set(undefined, "GenUI", stateKey(id), value),
          readOperation: (id) => get(undefined, operationKey(id)),
          writeOperation: (id, value) =>
            set(undefined, "GenUIResult", operationKey(id), value),
        },
        {
          copy: async (text) => {
            if (!navigator.clipboard)
              throw new Error("genui: clipboard access is unavailable");
            await navigator.clipboard.writeText(text);
          },
          open: (url) => {
            const link = document.createElement("a");
            link.href = safeURL(url);
            link.target = "_blank";
            link.rel = "noopener noreferrer";
            link.click();
          },
        },
      ),
    [context, presentation, initial, get, set],
  );
  const { action: view, inputIssue } = useSyncExternalStore(
    controller.subscribe,
    controller.snapshot,
    controller.snapshot,
  );
  const controls = useMemo(
    () => ({
      execute: (id: string) => controller.execute(id),
      reset: (fields?: string[]) => controller.reset(fields),
      resetForm: (id: string) =>
        controller.reset(presentation.formStateIds(id)),
      update: (id: string, value: unknown) => controller.update(id, value),
      readInitial: (id: string) => initial.get(id),
    }),
    [controller, initial, presentation],
  );
  useEffect(() => {
    void controller.initialize();
  }, [controller]);
  const inputFailure = inputIssue ? failureText(inputIssue.error, "") : "";
  const feedback = (
    <>
      {inputIssue && (
        <div role="alert">
          <p>{t("interaction.inputFailed")}</p>
          {inputFailure && <p>{inputFailure}</p>}
        </div>
      )}
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
              copied: t("code.copied"),
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
  return (
    <InteractionContext
      value={{
        blocked: controller.blocked,
        ...controls,
        feedback,
      }}
    >
      {children}
    </InteractionContext>
  );
}
