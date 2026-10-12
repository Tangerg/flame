import { useState } from "react";
import { Brain } from "@flame/icons/react";
import type { AgentMemoryItem, Page } from "@flame/runtime-contract/wire";
import {
  Badge,
  Checkbox,
  EmptyState,
  ListRow,
  Metadata,
  RecordBody,
  RecordGroup,
  SearchField,
  Select,
  Toolbar,
} from "@flame/ui";
import {
  mountView,
  Pagination,
  ReadStatus,
  useRead,
  ViewPanel,
  type ViewProps,
} from "../../src/view";

function Memory({ view }: ViewProps<Page<AgentMemoryItem>>) {
  const { data: page } = useRead(view);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [origin, setOrigin] = useState("all");
  const [pinned, setPinned] = useState(false);
  const rows = page.data;
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
    <ViewPanel
      view={view}
      title="Agent memory"
      description="Review proposed facts before they are recalled in future conversations. Manage memory with the controls above."
    >
      <Toolbar label="Memory filters">
        <SearchField
          id="search"
          aria-label="Search this page"
          placeholder="Recorded text or ID"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <Select
          label="Memory status"
          value={status}
          onValueChange={setStatus}
          options={[
            { value: "all", label: "All statuses" },
            { value: "pending", label: "Pending review" },
            { value: "active", label: "Active" },
          ]}
        />
        <Select
          label="Memory origin"
          value={origin}
          onValueChange={setOrigin}
          options={[
            { value: "all", label: "All origins" },
            { value: "auto", label: "Mined" },
            { value: "user", label: "User authored" },
          ]}
        />
        <Checkbox
          checked={pinned}
          onCheckedChange={setPinned}
          label="Pinned only"
        />
      </Toolbar>
      <Metadata>
        <span id="summary">{`${rows.filter((item) => item.status === "pending").length} pending · ${rows.filter((item) => item.status === "active").length} active on this page`}</span>
      </Metadata>
      <ReadStatus
        view={view}
        ready={`${rows.length} memory items loaded on this page.`}
        loading="Loading recorded memory…"
      />
      <RecordGroup id="items" aria-label="Recorded memory">
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
            key={item.id}
            data-memory-id={item.id}
            data-status={item.status}
          >
            <Badge tone={item.status === "pending" ? "accent" : "neutral"}>
              {item.status === "pending" ? "Pending review" : "Active"}
            </Badge>
            <RecordBody>{item.content}</RecordBody>
            <Metadata>
              {item.scope} ·{" "}
              {item.origin === "auto" ? "Mined" : "User authored"}
              {item.pinned ? " · Pinned" : ""}
            </Metadata>
            <Metadata>ID: {item.id}</Metadata>
            {item.sessionId && (
              <Metadata>Source Session: {item.sessionId}</Metadata>
            )}
            {item.day && <Metadata>Day: {item.day}</Metadata>}
          </ListRow>
        ))}
      </RecordGroup>
      <Pagination
        view={view}
        nextCursor={page.nextCursor}
        label="Memory pages"
      />
    </ViewPanel>
  );
}
mountView(Memory);
