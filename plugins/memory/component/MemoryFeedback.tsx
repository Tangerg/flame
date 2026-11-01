import {
  type ReactNode,
  type RefObject,
  useEffect,
  useLayoutEffect,
  useRef,
} from "react";
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
  const result = useRef<HTMLElement>(null);
  if (!feedback && !original) return null;
  const key = original && (
    <Metadata>
      {t("feedback.original", { key: original.promise.idempotencyKey })}
    </Metadata>
  );
  return (
    <RecordGroup>
      <RecordGroup
        ref={result}
        tabIndex={-1}
        role={feedback?.state === "refused" ? "alert" : "status"}
      >
        <RecordBody>
          {feedback &&
            t(`feedback.${feedback.state}`, { label: feedback.label })}
        </RecordBody>
        {feedback?.detail && <RecordBody>{feedback.detail}</RecordBody>}
      </RecordGroup>
      {original?.source === "record" ? (
        <Toolbar label={t("feedback.original.label")}>
          {key}
          <RecordRecovery result={result} pending={pending} onClick={reconcile}>
            {t("feedback.check")}
          </RecordRecovery>
        </Toolbar>
      ) : (
        key
      )}
    </RecordGroup>
  );
}

function RecordRecovery({
  result,
  pending,
  onClick,
  children,
}: {
  result: RefObject<HTMLElement | null>;
  pending: boolean;
  onClick(): void;
  children: ReactNode;
}) {
  const action = useRef<HTMLButtonElement>(null);
  const ownedFocus = useRef(false);
  useLayoutEffect(() => {
    const button = action.current;
    if (!button) return;
    ownedFocus.current = false;
    // Capture ownership before removal; passive cleanup sees the committed result.
    return () => {
      ownedFocus.current = button === button.ownerDocument.activeElement;
    };
  }, []);
  useEffect(() => {
    const button = action.current;
    const block = result.current;
    if (!button || !block) return;
    return () => {
      const active = block.ownerDocument.activeElement;
      if (
        !button.isConnected &&
        ownedFocus.current &&
        block.isConnected &&
        active === block.ownerDocument.body
      )
        block.focus();
    };
  }, [result]);
  return (
    <Button ref={action} pending={pending} onClick={onClick}>
      {children}
    </Button>
  );
}
