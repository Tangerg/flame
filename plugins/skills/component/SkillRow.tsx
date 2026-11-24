import { useRef, type ReactNode } from "react";
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
  read,
  feedback,
  children,
  t,
}: {
  item: SkillRecord;
  review: boolean;
  read?: { disabled: boolean; onOpen: (origin: HTMLElement) => void };
  feedback?: ReactNode;
  children?: ReactNode;
  t: Translate;
}) {
  const element = useRef<HTMLElement>(null);
  // The tab already names proposals and available Skills, and Active is the resting
  // lifecycle; only an archived Skill or a Skill's scope adds information.
  const status = review ? undefined : "lifecycle" in item ? (
    item.lifecycle === "archived" && <Badge>{t("lifecycle.archived")}</Badge>
  ) : "scope" in item ? (
    <Badge>{t(`scope.${item.scope}`)}</Badge>
  ) : undefined;
  const summary = (
    <RecordSummary
      title={item.name}
      description={item.description}
      status={status}
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
      <ListRow ref={element} data-focus-unit="">
        <RecordHeading
          title={item.name}
          description={item.description}
          status={status}
          actions={children}
        />
        {feedback}
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
