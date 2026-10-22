import {
  useEffect,
  useLayoutEffect,
  useRef,
  type ReactNode,
  type RefObject,
} from "react";
import { BookOpen } from "@flame/icons/react";
import { type Translate } from "@flame/component-sdk";

import {
  Badge,
  FormSection,
  Disclosure,
  Facts,
  ListRow,
  RecordBody,
  RecordButton,
  RecordHeading,
  RecordSummary,
} from "@flame/ui";
import { type SkillRecord } from "./types";
export function SkillRow({
  item,
  review,
  focusAfterRemoval,
  read,
  children,
  t,
}: {
  item: SkillRecord;
  review: boolean;
  focusAfterRemoval: RefObject<HTMLElement | null>;
  read?: { disabled: boolean; onOpen: (origin: HTMLElement) => void };
  children?: ReactNode;
  t: Translate;
}) {
  const element = useRef<HTMLElement>(null);
  const ownedFocus = useRef(false);
  useLayoutEffect(() => {
    const row = element.current;
    if (!row) return;
    ownedFocus.current = false;
    return () => {
      ownedFocus.current = row.contains(row.ownerDocument.activeElement);
    };
  }, []);
  useEffect(() => {
    const row = element.current;
    if (!row) return;
    return () => {
      const next = focusAfterRemoval.current;
      if (
        !row.isConnected &&
        ownedFocus.current &&
        row.ownerDocument.activeElement === row.ownerDocument.body &&
        next?.isConnected
      )
        next.focus();
    };
  }, [focusAfterRemoval]);
  const summary = (
    <RecordSummary
      icon={BookOpen}
      title={item.name}
      description={item.description}
      status={
        <Badge tone={review ? "warning" : "neutral"}>
          {review
            ? t("badge.proposal")
            : "lifecycle" in item
              ? t(`lifecycle.${item.lifecycle}`)
              : "scope" in item
                ? t(`scope.${item.scope}`)
                : t("badge.available")}
        </Badge>
      }
    />
  );
  if (read)
    return (
      <RecordButton
        ref={(node) => {
          element.current = node;
        }}
        disabled={read.disabled}
        onClick={(event) => read.onOpen(event.currentTarget)}
      >
        {summary}
      </RecordButton>
    );
  if ("lifecycle" in item)
    return (
      <ListRow ref={element}>
        <RecordHeading
          title={item.name}
          description={item.description}
          status={<Badge>{t(`lifecycle.${item.lifecycle}`)}</Badge>}
          actions={children}
        />
      </ListRow>
    );
  return (
    <Disclosure ref={element} variant="row" actions={children} title={summary}>
      {"instructions" in item && (
        <FormSection title={t("instructions.heading")}>
          <RecordBody>{item.instructions}</RecordBody>
        </FormSection>
      )}
      <Facts
        values={[
          [
            t("fact.scope"),
            "scope" in item ? t(`scope.${item.scope}`) : undefined,
          ],
          [t("fact.revision"), "revision" in item ? item.revision : undefined],
          [
            t("fact.installation"),
            "installation" in item
              ? item.installation?.installationId
              : undefined,
          ],
          [
            t("fact.release"),
            "installation" in item ? item.installation?.digest : undefined,
          ],
          [t("fact.origin"), "origin" in item ? item.origin : undefined],
          [
            t("fact.sourceSession"),
            "sourceSession" in item ? item.sourceSession : undefined,
          ],
          [
            t("fact.revises"),
            review
              ? String("revises" in item && item.revises === true)
              : undefined,
          ],
        ]}
      />
    </Disclosure>
  );
}
