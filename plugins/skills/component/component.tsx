import { useId, useRef } from "react";
import { ArrowLeft, BookOpen, Refresh } from "@flame/icons/react";
import { type ComponentContext, type Translate } from "@flame/component-sdk";

import {
  Button,
  Badge,
  Panel,
  PanelHeader,
  IconButton,
  PanelSection,
  RecordForm,
  FormSection,
  FormActions,
  DocumentBody,
  Disclosure,
  EmptyState,
  Facts,
  Icon,
  isImeKey,
  Metadata,
  PanelTitle,
  RecordBody,
  RecordGroup,
  RecordTitle,
  SearchField,
  FilterBar,
  Hang,
  Segmented,
  selectAllInReader,
  Toolbar,
} from "@flame/ui";
import {
  type CommandIntent,
  type Notice,
  type SkillSnapshot,
  type Target,
} from "./types";
import { SkillRow } from "./SkillRow";
import { useSkills, SECTION_VALUES } from "./useSkills";
const reviewPresentation = {
  approve: { variant: "primary" },
  reject: { variant: "tonal", tone: "negative" },
} as const;

function catalogSummary(
  snapshot: SkillSnapshot | undefined,
  matching: number,
  count: number,
  t: Translate,
) {
  if (!snapshot) return;
  const target = snapshot.type;
  return matching < count
    ? t(`summary.matching.${target}`, { matching, count })
    : t(`summary.${target}`, { count });
}

function commandLabel(intent: CommandIntent, t: Translate) {
  return t(`command.${intent.action}`, { name: intent.name });
}

function noticeText(notice: Notice, t: Translate) {
  switch (notice.kind) {
    case "completed":
      return t("notice.completed", { label: commandLabel(notice.intent, t) });
    case "revisionConflict":
      return t("notice.revisionConflict");
    case "failure":
      return notice.detail;
  }
}

function SkillCommandFeedback({
  command,
  notice,
  reconcile,
  t,
}: Pick<
  ReturnType<typeof useSkills>,
  "command" | "notice" | "reconcile" | "t"
>) {
  return (
    <>
      {notice && <p role="status">{noticeText(notice, t)}</p>}
      {command && (
        <section aria-label={t("command.status.label")}>
          <p role={command.phase === "uncertain" ? "alert" : "status"}>
            {command.phase === "uncertain"
              ? t("command.uncertain")
              : t("command.pending", {
                  label: commandLabel(command.command.intent, t),
                })}
          </p>
          {command.phase === "uncertain" && (
            <>
              <RecordBody>{command.error}</RecordBody>
              <Metadata>
                {t("command.original", {
                  key: command.command.promise.idempotencyKey,
                })}
              </Metadata>
              <Button onClick={reconcile}>{t("command.reconcile")}</Button>
            </>
          )}
        </section>
      )}
    </>
  );
}

function SkillReadStatus({
  opened,
  read,
  snapshot,
  skillDocument,
  title,
  observationError,
  t,
}: Pick<
  ReturnType<typeof useSkills>,
  | "opened"
  | "read"
  | "snapshot"
  | "skillDocument"
  | "title"
  | "observationError"
  | "t"
>) {
  return (
    <>
      {!opened && read.loading && <p role="status">{t("reading")}</p>}
      {!opened && read.error && (
        <p role="alert">
          {t("read.error", {
            catalog: title.toLocaleLowerCase(),
            error: read.error,
          })}
          {snapshot && ` ${t("read.retained")}`}
        </p>
      )}
      {opened?.loading && (
        <p role="status">{t("document.reading", { name: opened.name })}</p>
      )}
      {opened?.error && (
        <p role="alert">
          {skillDocument
            ? t("document.error.refresh", {
                name: opened.name,
                error: opened.error,
              })
            : t("document.error.read", {
                name: opened.name,
                error: opened.error,
              })}
        </p>
      )}
      {opened?.error && skillDocument && (
        <Metadata role="status">
          {t("document.retained", { name: opened.name })}
        </Metadata>
      )}
      {observationError && (
        <p role="alert">
          {t("observation.error", { error: observationError })}
        </p>
      )}
    </>
  );
}

function SkillDocumentToolbar({
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

function SkillCatalogControls({
  t,
  target,
  busy,
  loading,
  searchField,
  search,
  onNavigate,
  onSearch,
  onRefresh,
}: {
  t: Translate;
  target: Target;
  busy: boolean;
  loading: boolean;
  searchField: SkillsState["searchField"];
  search: string;
  onNavigate(target: Target): void;
  onSearch(value: string): void;
  onRefresh(): void;
}) {
  return (
    <>
      <Toolbar label={t("nav.label")}>
        <Hang>
          <Segmented
            ariaLabel={t("nav.catalog")}
            value={target}
            options={SECTION_VALUES.map((value) => ({
              value,
              label: t(`section.${value}`),
            }))}
            disabled={busy}
            onChange={(value) => onNavigate(value as Target)}
          />
        </Hang>
      </Toolbar>
      <FilterBar
        label={t("controls.label")}
        search={
          <SearchField
            ref={searchField}
            size="lg"
            id="search"
            aria-label={t("search.label")}
            placeholder={t("search.placeholder")}
            value={search}
            onChange={(event) => onSearch(event.target.value)}
            onClear={() => onSearch("")}
            clearLabel={t("search.clear")}
          />
        }
        actions={
          <IconButton
            icon={Refresh}
            aria-label={t("refresh")}
            size="md"
            disabled={busy}
            pending={loading}
            onClick={onRefresh}
          />
        }
      />
    </>
  );
}

function SkillReviewForm({
  t,
  ref,
  review,
  workspacePath,
  busy,
  disabled,
  onCancel,
  onConfirm,
}: {
  t: Translate;
  ref: SkillsState["reviewElement"];
  review: NonNullable<SkillsState["review"]>;
  workspacePath: string;
  busy: boolean;
  disabled: boolean;
  onCancel(): void;
  onConfirm(): void;
}) {
  return (
    <RecordForm ref={ref} aria-label={t("review.label")} data-focus-unit="">
      <RecordTitle>
        {t(`review.title.${review.decision}`, {
          name: review.proposal.name,
        })}
      </RecordTitle>
      <FormSection title={t("review.proposed")}>
        <RecordBody>{review.proposal.description}</RecordBody>
        <RecordBody>{review.proposal.instructions}</RecordBody>
      </FormSection>
      <Disclosure title={t("review.target")}>
        <Facts
          values={[
            [t("fact.workspace"), workspacePath],
            [t("fact.scope"), t(`scope.${review.proposal.scope}`)],
            [t("fact.revision"), review.proposal.revision],
          ]}
        />
      </Disclosure>
      <FormActions label={t("review.confirmation")}>
        <Button variant="ghost" disabled={busy} autoFocus onClick={onCancel}>
          {t("cancel")}
        </Button>
        <Button
          {...reviewPresentation[review.decision]}
          disabled={disabled}
          onClick={onConfirm}
        >
          {t(`confirm.${review.decision}`)}
        </Button>
      </FormActions>
    </RecordForm>
  );
}

type SkillsState = ReturnType<typeof useSkills>;

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
    searchField,
    search,
    searchCatalog,
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
  const feedbackIntent = command?.command.intent ?? notice?.intent;
  const feedbackName =
    feedbackIntent?.action === "archive" || feedbackIntent?.action === "restore"
      ? feedbackIntent.name
      : undefined;
  const feedbackRow =
    snapshot?.type === "library" &&
    matching.some((item) => item.name === feedbackName)
      ? feedbackName
      : undefined;
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
            scope={skillDocument?.scope}
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
        <>
          {skillDocument.description && (
            <RecordBody>{skillDocument.description}</RecordBody>
          )}
          <section
            ref={instructions}
            aria-label={t("instructions.for", { name: skillDocument.name })}
          >
            <DocumentBody>
              <RecordBody>{skillDocument.instructions}</RecordBody>
            </DocumentBody>
          </section>
          <Disclosure title={t("document.source")}>
            <Facts
              values={[
                [t("fact.sourcePath"), skillDocument.path],
                [t("fact.revision"), skillDocument.revision],
                [
                  t("fact.installation"),
                  skillDocument.installation?.installationId,
                ],
                [t("fact.release"), skillDocument.installation?.digest],
              ]}
            />
          </Disclosure>
        </>
      )}
      <div hidden={Boolean(opened)}>
        <PanelSection
          title={summary ?? title}
          description={
            target === "library"
              ? t("section.library.description")
              : !workspace && t("section.selectWorkspace")
          }
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
              <EmptyState
                icon={BookOpen}
                size="compact"
                title={
                  rows.length
                    ? t(
                        snapshot.type === "review"
                          ? "empty.review.filtered.title"
                          : "empty.filtered.title",
                      )
                    : t(
                        snapshot.type === "review"
                          ? "empty.review.none.title"
                          : "empty.none.title",
                      )
                }
                sub={
                  snapshot.type === "review"
                    ? undefined
                    : rows.length
                      ? t("empty.filtered.sub")
                      : t("empty.none.sub")
                }
              />
            )}
          </RecordGroup>
        </PanelSection>
      </div>
    </Panel>
  );
}
