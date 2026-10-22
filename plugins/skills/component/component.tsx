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
import { useSkills, sections } from "./useSkills";
export function Skills({ context }: { context: ComponentContext }) {
  const {
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
      label="Skills"
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
          title="Skills"
          description={
            opened
              ? undefined
              : target === "library"
                ? "Reusable instructions you manage."
                : "Instructions and proposals for this workspace."
          }
        />
      }
      controls={
        opened ? (
          <>
            <Toolbar
              label="Skill document"
              actions={
                <IconButton
                  icon={Refresh}
                  aria-label="Refresh document"
                  size="md"
                  pending={opened.loading}
                  onClick={() => void readDocument(opened.name)}
                />
              }
            >
              <Button
                variant="ghost"
                size="sm"
                aria-label="Back to available Skills"
                onClick={closeDocument}
              >
                <Icon glyph={ArrowLeft} size="sm" />
                Available Skills
              </Button>
              <PanelTitle ref={heading} tabIndex={-1} size="sm" aria-level={2}>
                {opened.name}
              </PanelTitle>
              {skillDocument && <Badge>{skillDocument.scope}</Badge>}
            </Toolbar>
          </>
        ) : (
          <>
            <Toolbar label="Skill navigation">
              <Segmented
                ariaLabel="Catalog"
                value={target}
                options={sections}
                disabled={Boolean(command)}
                onChange={(value) => navigate(value as Target)}
              />
            </Toolbar>
            <FilterBar
              label="Skill controls"
              search={
                <SearchField
                  ref={searchField}
                  size="lg"
                  id="search"
                  aria-label="Search this catalog"
                  placeholder="Name or description"
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  onClear={() => setSearch("")}
                  clearLabel="Clear search"
                />
              }
              actions={
                <IconButton
                  icon={Refresh}
                  aria-label="Refresh"
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
      {!opened && read.loading && <p role="status">Reading Skills…</p>}
      {!opened && read.error && (
        <p role="alert">
          Could not read {title.toLocaleLowerCase()}: {read.error}
          {snapshot && " The last accepted read is retained."}
        </p>
      )}
      {opened?.loading && <p role="status">Reading {opened.name}…</p>}
      {opened?.error && (
        <p role="alert">
          {skillDocument
            ? `Could not refresh ${opened.name}: ${opened.error}`
            : `Could not read ${opened.name}: ${opened.error}`}
        </p>
      )}
      {opened?.error && skillDocument && (
        <Metadata role="status">
          Showing the last accepted read of {opened.name}.
        </Metadata>
      )}
      {observationError && (
        <p role="alert">
          Live refresh is unavailable: {observationError}. Use Refresh to read
          the current owner.
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {command && (
        <section aria-label="Skill command status">
          <p role={command.uncertain ? "alert" : "status"}>
            {command.uncertain
              ? "The original command outcome is not confirmed. Reconcile it before issuing another command or changing catalogs."
              : `${command.label}…`}
          </p>
          {command.error && <RecordBody>{command.error}</RecordBody>}
          {command.uncertain && (
            <>
              <Metadata>
                Original command: {command.promise.idempotencyKey}
              </Metadata>
              <Button onClick={reconcile}>Reconcile original command</Button>
            </>
          )}
        </section>
      )}
      {review && (
        <RecordForm
          aria-label="Confirm proposal decision"
          opener={review.opener}
        >
          <RecordTitle>
            {review.decision === "approve" ? "Approve" : "Reject"} ·{" "}
            {review.proposal.name}
          </RecordTitle>
          <FormSection title="Proposed skill">
            <RecordBody>{review.proposal.description}</RecordBody>
            <RecordBody>{review.proposal.instructions}</RecordBody>
          </FormSection>
          <Disclosure title="Review target">
            <Facts
              values={[
                ["Workspace", workspace!.ref.path],
                ["Scope", review.proposal.scope],
                ["Revision", review.proposal.revision],
              ]}
            />
          </Disclosure>
          <FormActions label="Proposal confirmation">
            <Button
              variant="ghost"
              disabled={Boolean(command)}
              // oxlint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              onClick={() => setReview(undefined)}
            >
              Cancel
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
                  `${review.decision === "approve" ? "Approve" : "Reject"} ${name}`,
                );
              }}
            >
              Confirm {review.decision}
            </Button>
          </FormActions>
        </RecordForm>
      )}
      {skillDocument && (
        <>
          {skillDocument.description && (
            <RecordBody>{skillDocument.description}</RecordBody>
          )}
          <section aria-label={`Instructions for ${skillDocument.name}`}>
            <DocumentBody>
              <RecordBody>{skillDocument.instructions}</RecordBody>
            </DocumentBody>
          </section>
          <Disclosure title="Source and revision">
            <Facts
              values={[
                ["Source path", skillDocument.path],
                ["Revision", skillDocument.revision],
                ["Installation", skillDocument.installation?.installationId],
                ["Release", skillDocument.installation?.digest],
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
                ? "User-managed Skills. Installation-provided Skills are managed with their package."
                : workspace
                  ? `Workspace: ${workspace.ref.path}`
                  : "Select a workspace to inspect available Skills and proposals."}
            </>
          }
        >
          <Metadata role="status" className="sr-only">
            {snapshot
              ? `${rows.length} ${snapshot.type === "review" ? "pending proposals" : snapshot.type === "library" ? "user-managed Skills" : "available Skills"}`
              : "No accepted read."}
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
                        `${item.lifecycle === "archived" ? "Restore" : "Archive"} ${item.name}`,
                      )
                    }
                  >
                    {item.lifecycle === "archived" ? "Restore" : "Archive"}
                  </Button>
                )}
                {snapshot!.type === "review" && "origin" in item && (
                  <Toolbar label={`Proposal actions for ${item.name}`}>
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
                        {decision === "approve" ? "Approve" : "Reject"}
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
                    ? "No matching Skills in this catalog."
                    : "No Skills in this catalog."
                }
                sub={
                  rows.length
                    ? "Clear the search to show Skills."
                    : "Choose another catalog or refresh after adding a Skill."
                }
              />
            )}
          </RecordGroup>
        </PanelSection>
      </div>
    </Panel>
  );
}
