import { useId, useRef } from "react";
import { type ComponentContext } from "@flame/component-sdk";
import {
  Button,
  Panel,
  PanelHeader,
  PanelSection,
  isImeKey,
  Metadata,
  RecordBody,
  RecordGroup,
  selectAllInReader,
  Toolbar,
} from "@flame/ui";
import { SkillRow } from "./SkillRow";
import { useSkills } from "./useSkills";
import {
  SkillCatalogControls,
  SkillEmptyState,
  catalogDescription,
  catalogSummary,
} from "./catalog";
import {
  SkillCommandFeedback,
  SkillReadStatus,
  feedbackRowName,
} from "./feedback";
import { SkillDocument, SkillDocumentToolbar } from "./document";
import { SkillReviewForm } from "./review";

export function Skills({ context }: { context: ComponentContext }) {
  const {
    t,
    bodyViewport,
    opened,
    review,
    beginReview,
    closeReview,
    confirmReview,
    reviewElement,
    closeDocument,
    target,
    readDocument,
    skillDocument,
    command,
    navigate,
    search: { query: search, field: searchField, change: searchCatalog },
    read,
    refresh,
    title,
    snapshot,
    observationError,
    notice,
    reconcile,
    workspace,
    disabled,
    toggleArchived,
    rows,
    matching,
    openDocument,
  } = useSkills(context);
  const summary = catalogSummary(snapshot, matching.length, rows.length, t);
  const documentTitleId = useId();
  const instructions = useRef<HTMLElement>(null);
  const feedbackRow = feedbackRowName(command, notice, snapshot, matching);
  const feedback = (
    <SkillCommandFeedback
      command={command}
      notice={notice}
      reconcile={reconcile}
      t={t}
    />
  );
  return (
    <Panel
      bodyViewportRef={bodyViewport}
      bodyViewportLabelledBy={opened ? documentTitleId : undefined}
      label={t("panel.title")}
      onKeyDown={(event) => {
        if (instructions.current)
          selectAllInReader(event, instructions.current);
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          isImeKey(event.nativeEvent) ||
          (!opened && !review)
        )
          return;
        event.preventDefault();
        event.stopPropagation();
        if (review) closeReview();
        else closeDocument();
      }}
      header={<PanelHeader title={t("panel.title")} />}
      controls={
        opened ? (
          <SkillDocumentToolbar
            t={t}
            opened={opened}
            titleId={documentTitleId}
            onRefresh={() => void readDocument(opened.name)}
            onBack={closeDocument}
          />
        ) : (
          <SkillCatalogControls
            t={t}
            target={target}
            busy={Boolean(command)}
            loading={read.loading}
            searchField={searchField}
            search={search}
            onNavigate={navigate}
            onSearch={searchCatalog}
            onRefresh={() => void refresh()}
          />
        )
      }
    >
      <SkillReadStatus
        opened={opened}
        read={read}
        snapshot={snapshot}
        skillDocument={skillDocument}
        title={title}
        observationError={observationError}
        t={t}
      />
      {feedbackRow === undefined && feedback}
      {review && (
        <SkillReviewForm
          t={t}
          ref={reviewElement}
          review={review}
          workspacePath={workspace!.ref.path}
          busy={Boolean(command)}
          disabled={disabled}
          onCancel={closeReview}
          onConfirm={confirmReview}
        />
      )}
      {skillDocument && (
        <SkillDocument
          t={t}
          document={skillDocument}
          instructionsRef={instructions}
        />
      )}
      <div hidden={Boolean(opened)}>
        <PanelSection
          title={summary ?? title}
          description={catalogDescription(t, target, workspace)}
        >
          <Metadata role="status" className="sr-only">
            {summary ?? t("summary.none")}
          </Metadata>

          {snapshot?.type === "available" &&
            snapshot.data.diagnostics.map((diagnostic, index) => (
              <RecordBody key={index}>
                {diagnostic.name}: {diagnostic.detail}
              </RecordBody>
            ))}
          <RecordGroup
            variant="list"
            id="items"
            aria-label={title}
            hidden={!matching.length && (read.loading || !snapshot)}
          >
            {matching.map((item) => (
              <SkillRow
                key={
                  "revision" in item
                    ? item.revision
                    : `${snapshot!.type}:${item.name}`
                }
                item={item}
                review={snapshot!.type === "review"}
                t={t}
                read={
                  snapshot!.type === "available"
                    ? {
                        disabled: Boolean(command),
                        onOpen: (origin) => openDocument(item.name, origin),
                      }
                    : undefined
                }
                feedback={item.name === feedbackRow ? feedback : undefined}
              >
                {snapshot!.type === "library" && "lifecycle" in item && (
                  <Button
                    variant="soft"
                    size="md"
                    disabled={disabled}
                    onClick={() => toggleArchived(item)}
                  >
                    {t(
                      item.lifecycle === "archived"
                        ? "action.restore"
                        : "action.archive",
                    )}
                  </Button>
                )}
                {snapshot!.type === "review" && "origin" in item && (
                  <Toolbar label={t("proposal.actions", { name: item.name })}>
                    {(["reject", "approve"] as const).map((decision) => (
                      <Button
                        variant={decision === "approve" ? "outline" : "wash"}
                        tone={decision === "reject" ? "negative" : undefined}
                        size="md"
                        key={decision}
                        pending={disabled}
                        onClick={(event) =>
                          beginReview(item, decision, event.currentTarget)
                        }
                      >
                        {t(`action.${decision}`)}
                      </Button>
                    ))}
                  </Toolbar>
                )}
              </SkillRow>
            ))}
            {!matching.length && !read.loading && snapshot && (
              <SkillEmptyState
                t={t}
                type={snapshot.type}
                filtered={rows.length > 0}
              />
            )}
          </RecordGroup>
        </PanelSection>
      </div>
    </Panel>
  );
}
