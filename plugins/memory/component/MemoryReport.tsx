import { type ReactNode } from "react";
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
  feedback,
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
  feedback: ReactNode;
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
        title="Saved memory"
        description={
          page && (
            <span id="summary">{`${rows.filter((item) => item.Status === "pending").length} pending · ${rows.filter((item) => item.Status === "active").length} active on this page`}</span>
          )
        }
      >
        {feedback}
        {page && (
          <RecordGroup variant="list" id="items" aria-label="Recorded memory">
            {!matching.length && (
              <EmptyState
                icon={Brain}
                size="compact"
                title={
                  rows.length
                    ? "No matching items on this page."
                    : "No memory items in this target."
                }
                sub={
                  rows.length
                    ? "Clear the search and filters."
                    : "Use Add memory above to save a fact."
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
                      <span>Record details</span>
                      <Badge
                        tone={item.Status === "pending" ? "warning" : "neutral"}
                      >
                        {item.Status === "pending"
                          ? "Pending review"
                          : "Active"}
                      </Badge>
                      {item.Pinned && <Badge>Pinned</Badge>}
                    </RecordMeta>
                  }
                  actions={
                    <Toolbar label={`Actions for ${item.ID}`}>
                      {item.Status === "pending" ? (
                        <>
                          <Button
                            variant="soft"
                            size="md"
                            disabled={disabled}
                            onClick={() =>
                              command(
                                "flame.memory/review.v1",
                                { ID: item.ID, Decision: "approve" },
                                `Approve ${item.ID}`,
                              )
                            }
                          >
                            Approve
                          </Button>
                          <Button
                            variant="wash"
                            tone="negative"
                            size="md"
                            disabled={disabled}
                            onClick={() =>
                              command(
                                "flame.memory/review.v1",
                                { ID: item.ID, Decision: "reject" },
                                `Reject ${item.ID}`,
                              )
                            }
                          >
                            Reject
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
                            Edit
                          </Button>
                          <ActionMenu
                            label={`More actions for ${item.ID}`}
                            disabled={disabled}
                            items={[
                              {
                                label: item.Pinned ? "Unpin" : "Pin",
                                onSelect: () =>
                                  command(
                                    "flame.memory/update.v1",
                                    { ID: item.ID, Pinned: !item.Pinned },
                                    `${item.Pinned ? "Unpin" : "Pin"} ${item.ID}`,
                                  ),
                              },
                              {
                                label: "Delete",
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
                      ["ID", item.ID],
                      ["Scope", item.Scope],
                      [
                        "Origin",
                        item.Origin === "auto" ? "Mined" : "User authored",
                      ],
                      ["Source Session", item.SessionID || undefined],
                      ["Day", item.Day || undefined],
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
