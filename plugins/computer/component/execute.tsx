import { flushSync } from "react-dom";
import { useEffect, useId, useRef, useState, type RefObject } from "react";
import { failureText, type Translate } from "@flame/component-sdk";
import { parseJSON } from "@frame/sdk/json";
import {
  Button,
  isImeKey,
  Callout,
  Disclosure,
  Facts,
  FormField,
  Metadata,
  PanelSection,
  RecordBody,
  RecordForm,
  RecordGroup,
  Select,
  TextArea,
  Toolbar,
} from "@flame/ui";
import type { ExecuteInput, Surface } from "./types";
import { LIMITS } from "../contracts/limits";
import { Source } from "./result";
import { ENGINE_TOOLS } from "../contracts/tools";
import { type ComputerView } from "./controls";

function ExecuteForm({
  surface,
  ready,
  pending,
  execute,
  editor,
  fallbackFocus,
  t,
}: {
  surface: Surface | undefined;
  ready: boolean;
  pending: boolean;
  execute(input: ExecuteInput): Promise<boolean>;
  editor: RefObject<HTMLTextAreaElement | null>;
  fallbackFocus: RefObject<HTMLElement | null>;
  t: Translate;
}) {
  const [tool, setTool] = useState<ExecuteInput["tool"]>("js");
  const [argumentsJSON, setArgumentsJSON] = useState("{}");
  const [review, setReview] = useState<ExecuteInput>();
  const [error, setError] = useState<string>();
  const errorId = useId();
  const cancel = useRef<HTMLButtonElement>(null);
  const reviewElement = useRef<HTMLElement>(null);
  useEffect(() => {
    if (review) cancel.current?.focus();
  }, [review]);
  const current =
    review &&
    surface &&
    review.sessionId === surface.sessionId &&
    review.workerId === surface.workerId &&
    review.surfaceDigest === surface.surfaceDigest;
  const editing = review === undefined && surface !== undefined;

  function closeReview() {
    const form = reviewElement.current;
    const ownedFocus = Boolean(
      form?.contains(form.ownerDocument.activeElement),
    );
    flushSync(() => setReview(undefined));
    if (ownedFocus)
      (surface ? editor.current : fallbackFocus.current)?.focus({
        preventScroll: true,
      });
  }

  function prepare() {
    if (!surface || !ready || pending) return;
    setError(undefined);
    try {
      const value = parseJSON(argumentsJSON);
      if (value === null || typeof value !== "object" || Array.isArray(value))
        throw new Error(t("execute.object"));
      setReview({
        sessionId: surface.sessionId,
        workerId: surface.workerId,
        surfaceDigest: surface.surfaceDigest,
        tool,
        argumentsJSON,
      });
    } catch (cause) {
      setError(failureText(cause));
      editor.current?.focus();
    }
  }

  return (
    <RecordGroup hidden={!surface && !review}>
      <Metadata>{t("execute.help")}</Metadata>
      {review && (
        <RecordForm
          ref={reviewElement}
          role="region"
          aria-label={t("execute.review")}
          onKeyDown={(event) => {
            if (
              event.key === "Escape" &&
              !event.defaultPrevented &&
              !pending &&
              !isImeKey(event.nativeEvent)
            ) {
              event.preventDefault();
              event.stopPropagation();
              closeReview();
            }
          }}
        >
          <Callout kind="warning">
            {t(
              review.tool === "js_reset"
                ? "execute.resetWarning"
                : "execute.warning",
            )}
          </Callout>
          <Facts
            values={[
              [t("session.id"), review.sessionId],
              [t("session.worker"), review.workerId],
              [t("execute.tool"), review.tool],
              [t("session.surface"), review.surfaceDigest],
            ]}
          />
          <Source
            value={review.argumentsJSON}
            label={t("execute.arguments")}
            t={t}
          />
          {surface && !current && (
            <Callout kind="warning">{t("execute.stale")}</Callout>
          )}
          <Toolbar label={t("execute.actions")}>
            <Button
              ref={cancel}
              disabled={pending}
              onClick={() => {
                closeReview();
              }}
            >
              {t("execute.back")}
            </Button>
            <Button
              variant="primary"
              disabled={!ready || pending || !current}
              onClick={() => {
                const captured = review;
                closeReview();
                void execute(captured);
              }}
            >
              {t("execute.confirm")}
            </Button>
          </Toolbar>
        </RecordForm>
      )}
      <RecordGroup hidden={!editing}>
        <Select
          label={t("execute.tool")}
          value={tool}
          options={ENGINE_TOOLS.map((value) => ({ value, label: value }))}
          disabled={!ready || pending}
          onValueChange={(value) => {
            setTool(value as ExecuteInput["tool"]);
            setArgumentsJSON("{}");
          }}
        />
        <FormField label={t("execute.arguments")}>
          <TextArea
            ref={editor}
            font="mono"
            value={argumentsJSON}
            onChange={(event) => {
              setArgumentsJSON(event.target.value);
              setError(undefined);
            }}
            rows={8}
            spellCheck={false}
            maxLength={LIMITS.maxInputBytes}
            readOnly={!ready || pending}
            invalid={error !== undefined}
            aria-describedby={error ? errorId : undefined}
          />
        </FormField>
        {error && (
          <div id={errorId} role="alert">
            <RecordBody>{error}</RecordBody>
          </div>
        )}
        <Button disabled={!ready || pending} onClick={prepare}>
          {t("execute.review")}
        </Button>
      </RecordGroup>
      {surface && (
        <>
          <Disclosure title={t("context.instructions")}>
            <Source
              t={t}
              value={surface.instructions}
              label={t("context.instructions")}
            />
          </Disclosure>
          <Disclosure title={t("context.tools")}>
            <Source
              value={surface.toolsJSON}
              label={t("context.tools")}
              t={t}
            />
          </Disclosure>
          <Disclosure title={t("context.server")}>
            <Source
              value={surface.serverJSON}
              label={t("context.server")}
              t={t}
            />
          </Disclosure>
        </>
      )}
    </RecordGroup>
  );
}

export function ManualExecution({
  surface,
  ready,
  pending,
  execute,
  editor,
  fallbackFocus,
  t,
}: {
  surface: ComputerView["surface"];
  ready: boolean;
  pending: boolean;
  execute(input: ExecuteInput): Promise<boolean>;
  editor: RefObject<HTMLTextAreaElement | null>;
  fallbackFocus: RefObject<HTMLElement | null>;
  t: Translate;
}) {
  return (
    <PanelSection title={t("execute.title")}>
      {surface?.loading && <p role="status">{t("context.reading")}</p>}
      {surface?.error && (
        <div role="alert">
          <RecordBody>{surface.error}</RecordBody>
        </div>
      )}
      <ExecuteForm
        surface={surface?.value}
        ready={
          ready &&
          surface?.value !== undefined &&
          !surface.loading &&
          !surface.error
        }
        pending={pending}
        execute={execute}
        editor={editor}
        fallbackFocus={fallbackFocus}
        t={t}
      />
    </PanelSection>
  );
}
