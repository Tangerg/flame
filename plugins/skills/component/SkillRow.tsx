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
// The tab already names proposals and available Skills, and Active is the resting
// lifecycle; only an archived Skill or a Skill's scope adds information.
function skillStatus(item: SkillRecord, t: Translate): ReactNode {
  if ("lifecycle" in item)
    return (
      item.lifecycle === "archived" && <Badge>{t("lifecycle.archived")}</Badge>
    );
  if ("scope" in item) return <Badge>{t(`scope.${item.scope}`)}</Badge>;
  return undefined;
}

// Each record kind carries a different subset of these facts; absent ones stay empty.
function skillFacts(
  item: SkillRecord,
  review: boolean,
  t: Translate,
): Array<[string, string | undefined]> {
  const installation = "installation" in item ? item.installation : undefined;
  return [
    [t("fact.scope"), "scope" in item ? t(`scope.${item.scope}`) : undefined],
    [t("fact.revision"), "revision" in item ? item.revision : undefined],
    [t("fact.installation"), installation?.installationId],
    [t("fact.release"), installation?.digest],
    [t("fact.origin"), "origin" in item ? item.origin : undefined],
    [
      t("fact.sourceSession"),
      "sourceSession" in item ? item.sourceSession : undefined,
    ],
    [
      t("fact.revises"),
      review ? String("revises" in item && item.revises === true) : undefined,
    ],
  ];
}

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
  const status = review ? undefined : skillStatus(item, t);
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
      <Facts values={skillFacts(item, review, t)} />
    </Disclosure>
  );
}
