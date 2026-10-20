import {
  useEffect,
  useLayoutEffect,
  useRef,
  type ReactNode,
  type RefObject,
} from "react";
import { BookOpen } from "@flame/icons/react";

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
}: {
  item: SkillRecord;
  review: boolean;
  focusAfterRemoval: RefObject<HTMLElement | null>;
  read?: { disabled: boolean; onOpen: (origin: HTMLElement) => void };
  children?: ReactNode;
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
            ? "Proposal"
            : "lifecycle" in item
              ? item.lifecycle
              : "scope" in item
                ? item.scope
                : "Available"}
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
          status={<Badge>{item.lifecycle}</Badge>}
          actions={children}
        />
      </ListRow>
    );
  return (
    <Disclosure ref={element} variant="row" actions={children} title={summary}>
      {"instructions" in item && (
        <FormSection title="Instructions">
          <RecordBody>{item.instructions}</RecordBody>
        </FormSection>
      )}
      <Facts
        values={[
          ["Scope", "scope" in item ? item.scope : undefined],
          ["Revision", "revision" in item ? item.revision : undefined],
          [
            "Installation",
            "installation" in item
              ? item.installation?.installationId
              : undefined,
          ],
          [
            "Release",
            "installation" in item ? item.installation?.digest : undefined,
          ],
          ["Origin", "origin" in item ? item.origin : undefined],
          [
            "Source Session",
            "sourceSession" in item ? item.sourceSession : undefined,
          ],
          [
            "Revises an existing Skill",
            review
              ? String("revises" in item && item.revises === true)
              : undefined,
          ],
        ]}
      />
    </Disclosure>
  );
}
