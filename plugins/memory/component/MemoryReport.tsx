import { type ReactNode, type Ref, type RefObject, useState } from "react";
import { type Translate } from "@flame/component-sdk";
import { type MethodInputs } from "../package/io.github.tangerg.flame/contracts/methods.generated";
import { Brain } from "@flame/icons/react";
import {
  Button,
  PanelSection,
  ActionMenu,
  Disclosure,
  Facts,
  Badge,
  EmptyState,
  ListRow,
  RecordBody,
  RecordMeta,
  RecordGroup,
  Toolbar,
} from "@flame/ui";
import { MemoryAction } from "./MemoryAction";
import {
  type CommandIntent,
  type MemoryItem,
  type MemoryPage,
  type MutationName,
} from "./types";
type RecordCommand = <K extends MutationName>(
  method: K,
  input: MethodInputs[K],
  intent: CommandIntent,
) => void;
export function MemoryReport({
  page,
  pageNumber,
  summary,
  disabled,
  authoring,
  edit,
  remove,
  command,
  search,
  status,
  origin,
  pinned,
  clearSearchAndFilters,
  feedback,
  resultFocus,
  canRestoreFocus,
  t,
}: {
  page?: MemoryPage;
  pageNumber?: number;
  summary: Ref<HTMLSpanElement>;
  disabled: boolean;
  authoring: boolean;
  edit(item: MemoryItem, opener: HTMLElement): void;
  remove(item: MemoryItem, opener: HTMLElement): void;
  command: RecordCommand;
  search: string;
  status: string;
  origin: string;
  pinned: boolean;
  clearSearchAndFilters(): void;
  feedback: ReactNode;
  resultFocus: RefObject<HTMLElement | null>;
  canRestoreFocus(): boolean;
  t: Translate;
}) {
  const rows = page?.rows ?? [];
  const matching = rows.filter(
    (item) =>
      (status === "all" || item.status === status) &&
      (origin === "all" || item.origin === origin) &&
      (!pinned || item.pinned) &&
      `${item.id} ${item.content} ${item.sessionId ?? ""}`
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
  );
  return (
    <>
      <PanelSection
        title={t("catalog.title")}
        description={
          page &&
          pageNumber !== undefined && (
            <span id="summary" ref={summary} tabIndex={-1}>
              {(pageNumber > 1 || Boolean(page.nextCursor)) &&
                `${t("footer.page", { page: pageNumber })} · `}
              {t("footer.items", { count: rows.length })}
              {" · "}
              {t("catalog.pending", {
                count: rows.filter((item) => item.status === "pending").length,
              })}
              {" · "}
              {t("catalog.active", {
                count: rows.filter((item) => item.status === "active").length,
              })}
            </span>
          )
        }
      >
        {feedback}
        {page && (
          <RecordGroup
            variant="list"
            id="items"
            aria-label={t("catalog.group.label")}
          >
            {!matching.length && (
              <EmptyState
                icon={Brain}
                size="compact"
                title={
                  rows.length
                    ? t("catalog.empty.filtered.title")
                    : t("catalog.empty.none.title")
                }
                sub={rows.length ? undefined : t("catalog.empty.none.sub")}
                action={
                  rows.length ? (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={clearSearchAndFilters}
                    >
                      {t("catalog.empty.filtered.action")}
                    </Button>
                  ) : undefined
                }
              />
            )}
            {matching.map((item) => (
              <ListRow
                key={item.id}
                data-memory-id={item.id}
                data-status={item.status}
              >
                <RecordBody>{item.content}</RecordBody>
                <Disclosure
                  title={
                    <RecordMeta>
                      <span>{t("record.details")}</span>
                      {/* Active is the resting state; only review and pinning need a word. */}
                      {item.status === "pending" && (
                        <Badge tone="warning">{t("status.pending")}</Badge>
                      )}
                      {item.pinned && <Badge>{t("pinned")}</Badge>}
                    </RecordMeta>
                  }
                  actions={
                    <Toolbar label={t("actions.for", { id: item.id })}>
                      {item.status === "pending" ? (
                        <>
                          <MemoryAction
                            result={resultFocus}
                            canRestoreFocus={canRestoreFocus}
                            variant="soft"
                            size="md"
                            disabled={disabled}
                            focusableWhenDisabled
                            onClick={() =>
                              command(
                                "flame.memory/review.v3",
                                {
                                  id: item.id,
                                  expectedRevision: item.revision,
                                  decision: "approve",
                                },
                                { action: "approve", id: item.id },
                              )
                            }
                          >
                            {t("approve")}
                          </MemoryAction>
                          <MemoryAction
                            result={resultFocus}
                            canRestoreFocus={canRestoreFocus}
                            variant="wash"
                            tone="negative"
                            size="md"
                            disabled={disabled}
                            focusableWhenDisabled
                            onClick={() =>
                              command(
                                "flame.memory/review.v3",
                                {
                                  id: item.id,
                                  expectedRevision: item.revision,
                                  decision: "reject",
                                },
                                { action: "reject", id: item.id },
                              )
                            }
                          >
                            {t("reject")}
                          </MemoryAction>
                        </>
                      ) : (
                        <>
                          <Button
                            variant="soft"
                            size="md"
                            disabled={disabled || authoring}
                            focusableWhenDisabled
                            onClick={(event) => edit(item, event.currentTarget)}
                          >
                            {t("edit.action")}
                          </Button>
                          <MemoryMenu
                            item={item}
                            disabled={disabled}
                            authoring={authoring}
                            result={resultFocus}
                            command={command}
                            remove={remove}
                            t={t}
                          />
                        </>
                      )}
                    </Toolbar>
                  }
                >
                  <Facts
                    values={[
                      [t("fact.id"), item.id],
                      [t("fact.scope"), t(`scope.${item.scope}`)],
                      [
                        t("fact.origin"),
                        item.origin === "auto"
                          ? t("origin.auto")
                          : t("origin.user"),
                      ],
                      [t("fact.session"), item.sessionId || undefined],
                      [t("fact.day"), item.day || undefined],
                    ]}
                  />
                </Disclosure>
              </ListRow>
            ))}
          </RecordGroup>
        )}
      </PanelSection>
    </>
  );
}

function MemoryMenu({
  item,
  disabled,
  authoring,
  result,
  command,
  remove,
  t,
}: {
  item: MemoryItem;
  disabled: boolean;
  authoring: boolean;
  result: RefObject<HTMLElement | null>;
  command: RecordCommand;
  remove(item: MemoryItem, opener: HTMLElement): void;
  t: Translate;
}) {
  const [selectedCommand, setSelectedCommand] = useState(false);
  return (
    <ActionMenu
      label={t("actions.more", { id: item.id })}
      disabled={disabled}
      finalFocus={
        selectedCommand
          ? () => {
              // Settlement can finish before the popup closes; selection owns this handoff.
              setSelectedCommand(false);
              return result.current ?? true;
            }
          : undefined
      }
      items={[
        {
          label: item.pinned ? t("action.unpin") : t("action.pin"),
          onSelect: () => {
            setSelectedCommand(true);
            command(
              "flame.memory/update.v3",
              {
                id: item.id,
                expectedRevision: item.revision,
                pinned: !item.pinned,
              },
              { action: item.pinned ? "unpin" : "pin", id: item.id },
            );
          },
        },
        {
          label: t("action.delete"),
          tone: "negative",
          disabled: authoring,
          onSelect: (opener) => remove(item, opener),
        },
      ]}
    />
  );
}
