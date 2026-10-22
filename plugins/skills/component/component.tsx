import { ArrowLeft, BookOpen, Refresh } from "@flame/icons/react";
import { type ComponentContext } from "@flame/component-sdk";

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
  Segmented,
  Toolbar,
} from "@flame/ui";
import { type Target } from "./types";
import { SkillRow } from "./SkillRow";
import { useSkills, SECTION_VALUES } from "./useSkills";
export function Skills({ context }: { context: ComponentContext }) {
  const {
    t,
    panel,
    opened,
    review,
    setReview,
    closeDocument,
    target,
    readDocument,
    heading,
    skillDocument,
    command,
    navigate,
    searchField,
    search,
    setSearch,
    read,
    refresh,
    title,
    snapshot,
    observationError,
    notice,
    reconcile,
    workspace,
    disabled,
    submit,
    rows,
    matching,
    openDocument,
  } = useSkills(context);
  return (
    <Panel
      ref={panel}
      label={t("panel.title")}
      onKeyDown={(event) => {
        if (
          event.key !== "Escape" ||
          event.defaultPrevented ||
          isImeKey(event.nativeEvent) ||
          (!opened && !review)
        )
          return;
        event.preventDefault();
        event.stopPropagation();
        if (review) setReview(undefined);
        else closeDocument();
      }}
      header={
        <PanelHeader
          title={t("panel.title")}
          description={
            opened
              ? undefined
              : target === "library"
                ? t("header.library")
                : t("header.workspace")
          }
        />
      }
      controls={
        opened ? (
          <>
            <Toolbar
              label={t("document.toolbar")}
              actions={
                <IconButton
                  icon={Refresh}
                  aria-label={t("document.refresh")}
                  size="md"
                  pending={opened.loading}
                  onClick={() => void readDocument(opened.name)}
                />
              }
            >
              <Button
                variant="ghost"
                size="sm"
                aria-label={t("document.back")}
                onClick={closeDocument}
              >
                <Icon glyph={ArrowLeft} size="sm" />
                {t("section.available")}
              </Button>
              <PanelTitle ref={heading} tabIndex={-1} size="sm" aria-level={2}>
                {opened.name}
              </PanelTitle>
              {skillDocument && <Badge>{skillDocument.scope}</Badge>}
            </Toolbar>
          </>
        ) : (
          <>
            <Toolbar label={t("nav.label")}>
              <Segmented
                ariaLabel={t("nav.catalog")}
                value={target}
                options={SECTION_VALUES.map((value) => ({
                  value,
                  label: t(`section.${value}`),
                }))}
                disabled={Boolean(command)}
                onChange={(value) => navigate(value as Target)}
              />
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
                  onChange={(event) => setSearch(event.target.value)}
                  onClear={() => setSearch("")}
                  clearLabel={t("search.clear")}
                />
              }
              actions={
                <IconButton
                  icon={Refresh}
                  aria-label={t("refresh")}
                  size="md"
                  disabled={Boolean(command)}
                  pending={read.loading}
                  onClick={() => void refresh()}
                />
              }
            />
          </>
        )
      }
    >
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
      {notice && <p role="status">{notice}</p>}
      {command && (
        <section aria-label={t("command.status.label")}>
          <p role={command.uncertain ? "alert" : "status"}>
            {command.uncertain
              ? t("command.uncertain")
              : t("command.pending", { label: command.label })}
          </p>
          {command.error && <RecordBody>{command.error}</RecordBody>}
          {command.uncertain && (
            <>
              <Metadata>
                {t("command.original", {
                  key: command.promise.idempotencyKey,
                })}
              </Metadata>
              <Button onClick={reconcile}>{t("command.reconcile")}</Button>
            </>
          )}
        </section>
      )}
      {review && (
        <RecordForm aria-label={t("review.label")} opener={review.opener}>
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
                [t("fact.workspace"), workspace!.ref.path],
                [t("fact.scope"), review.proposal.scope],
                [t("fact.revision"), review.proposal.revision],
              ]}
            />
          </Disclosure>
          <FormActions label={t("review.confirmation")}>
            <Button
              variant="ghost"
              disabled={Boolean(command)}
              // oxlint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              onClick={() => setReview(undefined)}
            >
              {t("cancel")}
            </Button>
            <Button
              variant={review.decision === "approve" ? "primary" : "tonal"}
              tone={review.decision === "reject" ? "negative" : undefined}
              disabled={disabled}
              onClick={() => {
                const { scope, name, revision } = review.proposal;
                submit(
                  () =>
                    review.decision === "approve"
                      ? workspace!.skills.approveProposal({
                          scope,
                          name,
                          revision,
                        })
                      : workspace!.skills.rejectProposal({
                          scope,
                          name,
                          revision,
                        }),
                  t(`command.${review.decision}`, { name }),
                );
              }}
            >
              {t(`confirm.${review.decision}`)}
            </Button>
          </FormActions>
        </RecordForm>
      )}
      {skillDocument && (
        <>
          {skillDocument.description && (
            <RecordBody>{skillDocument.description}</RecordBody>
          )}
          <section
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
          title={
            <>
              {title} {snapshot && <Badge>{rows.length}</Badge>}
            </>
          }
          description={
            <>
              {target === "library"
                ? t("section.library.description")
                : workspace
                  ? t("section.workspace.path", { path: workspace.ref.path })
                  : t("section.selectWorkspace")}
            </>
          }
        >
          <Metadata role="status" className="sr-only">
            {snapshot
              ? t(`summary.${snapshot.type}`, { count: rows.length })
              : t("summary.none")}
          </Metadata>

          {snapshot?.type === "available" &&
            snapshot.data.diagnostics.map((diagnostic, index) => (
              <RecordBody key={index}>
                {diagnostic.name}: {diagnostic.detail}
              </RecordBody>
            ))}
          <RecordGroup variant="list" id="items" aria-label={title}>
            {matching.map((item) => (
              <SkillRow
                key={`${snapshot!.type}:${"scope" in item ? `${item.scope}:${item.name}` : item.name}`}
                item={item}
                review={snapshot!.type === "review"}
                focusAfterRemoval={searchField}
                t={t}
                read={
                  snapshot!.type === "available"
                    ? {
                        disabled: Boolean(command),
                        onOpen: (origin) => openDocument(item.name, origin),
                      }
                    : undefined
                }
              >
                {snapshot!.type === "library" && "lifecycle" in item && (
                  <Button
                    variant="soft"
                    size="md"
                    disabled={disabled}
                    onClick={() =>
                      submit(
                        () =>
                          item.lifecycle === "archived"
                            ? context.runtime.skills.restore(item.name)
                            : context.runtime.skills.archive(item.name),
                        t(
                          item.lifecycle === "archived"
                            ? "command.restore"
                            : "command.archive",
                          { name: item.name },
                        ),
                      )
                    }
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
                          setReview({
                            proposal: structuredClone(item),
                            decision,
                            opener: event.currentTarget,
                          })
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
                    ? t("empty.filtered.title")
                    : t("empty.none.title")
                }
                sub={
                  rows.length ? t("empty.filtered.sub") : t("empty.none.sub")
                }
              />
            )}
          </RecordGroup>
        </PanelSection>
      </div>
    </Panel>
  );
}
