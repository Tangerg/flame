import { type RefObject } from "react";
import { ArrowLeft, Refresh } from "@flame/icons/react";
import { type Translate } from "@flame/component-sdk";
import {
  Button,
  Badge,
  IconButton,
  DocumentBody,
  Disclosure,
  Facts,
  Icon,
  PanelTitle,
  RecordBody,
  Toolbar,
} from "@flame/ui";
import { type SkillDetail } from "@flame/runtime-client/wire";
import { type SkillsState } from "./useSkills";

export function SkillDocumentToolbar({
  t,
  opened,
  titleId,
  scope,
  onRefresh,
  onBack,
}: {
  t: Translate;
  opened: NonNullable<SkillsState["opened"]>;
  titleId: string;
  scope: string | undefined;
  onRefresh(): void;
  onBack(): void;
}) {
  return (
    <Toolbar
      label={t("document.toolbar")}
      actions={
        <IconButton
          icon={Refresh}
          aria-label={t("document.refresh")}
          size="md"
          pending={opened.loading}
          onClick={onRefresh}
        />
      }
    >
      <Button
        variant="ghost"
        size="sm"
        aria-label={t("document.back")}
        onClick={onBack}
      >
        <Icon glyph={ArrowLeft} size="sm" />
        {t("section.available")}
      </Button>
      <PanelTitle id={titleId} size="sm" aria-level={2}>
        {opened.name}
      </PanelTitle>
      {scope && <Badge>{t(`scope.${scope}`)}</Badge>}
    </Toolbar>
  );
}

export function SkillDocument({
  t,
  document,
  instructionsRef,
}: {
  t: Translate;
  document: SkillDetail;
  instructionsRef: RefObject<HTMLElement | null>;
}) {
  return (
    <>
      {document.description && <RecordBody>{document.description}</RecordBody>}
      <section
        ref={instructionsRef}
        aria-label={t("instructions.for", { name: document.name })}
      >
        <DocumentBody>
          <RecordBody>{document.instructions}</RecordBody>
        </DocumentBody>
      </section>
      <Disclosure title={t("document.source")}>
        <Facts
          values={[
            [t("fact.sourcePath"), document.path],
            [t("fact.revision"), document.revision],
            [t("fact.installation"), document.installation?.installationId],
            [t("fact.release"), document.installation?.digest],
          ]}
        />
      </Disclosure>
    </>
  );
}
