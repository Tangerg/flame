import { type ReactNode } from "react";
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
import { type MemoryItem, type MemoryPage, type MutationName } from "./types";
export function MemoryReport({
  page,
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
  t,
}: {
  page?: MemoryPage;
  disabled: boolean;
  authoring: boolean;
  edit(item: MemoryItem, opener: HTMLElement): void;
  remove(item: MemoryItem, opener: HTMLElement): void;
  command<K extends MutationName>(
    method: K,
    input: MethodInputs[K],
    label: string,
  ): void;
  search: string;
  status: string;
  origin: string;
  pinned: boolean;
  clearSearchAndFilters(): void;
  feedback: ReactNode;
  t: Translate;
}) {
  const rows = page?.Rows ?? [];
  const matching = rows.filter(
    (item) =>
      (status === "all" || item.Status === status) &&
      (origin === "all" || item.Origin === origin) &&
      (!pinned || item.Pinned) &&
      `${item.ID} ${item.Content} ${item.SessionID ?? ""}`
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
  );
  return (
    <>
      <PanelSection
        title={t("catalog.title")}
        description={
          page && (
            <span id="summary">
              {t("catalog.summary", {
                pending: rows.filter((item) => item.Status === "pending")
                  .length,
                active: rows.filter((item) => item.Status === "active").length,
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
                key={item.ID}
                data-memory-id={item.ID}
                data-status={item.Status}
              >
                <RecordBody>{item.Content}</RecordBody>
                <Disclosure
                  title={
                    <RecordMeta>
                      <span>{t("record.details")}</span>
                      <Badge
                        tone={item.Status === "pending" ? "warning" : "neutral"}
                      >
                        {item.Status === "pending"
                          ? t("status.pending")
                          : t("status.active")}
                      </Badge>
                      {item.Pinned && <Badge>{t("pinned")}</Badge>}
                    </RecordMeta>
                  }
                  actions={
                    <Toolbar label={t("actions.for", { id: item.ID })}>
                      {item.Status === "pending" ? (
                        <>
                          <Button
                            variant="soft"
                            size="md"
                            disabled={disabled}
                            onClick={() =>
                              command(
                                "flame.memory/review.v2",
                                {
                                  ID: item.ID,
                                  ExpectedRevision: item.Revision,
                                  Decision: "approve",
                                },
                                t("command.approve", { id: item.ID }),
                              )
                            }
                          >
                            {t("approve")}
                          </Button>
                          <Button
                            variant="wash"
                            tone="negative"
                            size="md"
                            disabled={disabled}
                            onClick={() =>
                              command(
                                "flame.memory/review.v2",
                                {
                                  ID: item.ID,
                                  ExpectedRevision: item.Revision,
                                  Decision: "reject",
                                },
                                t("command.reject", { id: item.ID }),
                              )
                            }
                          >
                            {t("reject")}
                          </Button>
                        </>
                      ) : (
                        <>
                          <Button
                            variant="soft"
                            size="md"
                            disabled={disabled || authoring}
                            onClick={(event) => edit(item, event.currentTarget)}
                          >
                            {t("edit.action")}
                          </Button>
                          <ActionMenu
                            label={t("actions.more", { id: item.ID })}
                            disabled={disabled}
                            items={[
                              {
                                label: item.Pinned
                                  ? t("action.unpin")
                                  : t("action.pin"),
                                onSelect: () =>
                                  command(
                                    "flame.memory/update.v2",
                                    {
                                      ID: item.ID,
                                      ExpectedRevision: item.Revision,
                                      Pinned: !item.Pinned,
                                    },
                                    item.Pinned
                                      ? t("command.unpin", { id: item.ID })
                                      : t("command.pin", { id: item.ID }),
                                  ),
                              },
                              {
                                label: t("action.delete"),
                                tone: "negative",
                                disabled: authoring,
                                onSelect: (opener) => remove(item, opener),
                              },
                            ]}
                          />
                        </>
                      )}
                    </Toolbar>
                  }
                >
                  <Facts
                    values={[
                      [t("fact.id"), item.ID],
                      [t("fact.scope"), item.Scope],
                      [
                        t("fact.origin"),
                        item.Origin === "auto"
                          ? t("origin.auto")
                          : t("origin.user"),
                      ],
                      [t("fact.session"), item.SessionID || undefined],
                      [t("fact.day"), item.Day || undefined],
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
