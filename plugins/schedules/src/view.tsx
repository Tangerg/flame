import { useState } from "react";
import { CalendarClock } from "@flame/icons/react";
import type { Page, Schedule } from "@flame/runtime-contract/wire";
import {
  Badge,
  EmptyState,
  Facts,
  ListRow,
  Metadata,
  RecordBody,
  RecordGroup,
  RecordTitle,
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

function Schedules({ view }: ViewProps<Page<Schedule>>) {
  const { data: page } = useRead(view);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const rows = page.data;
  const matching = rows.filter(
    (item) =>
      (status === "all" ||
        (item.enabled ? "enabled" : "disabled") === status) &&
      `${item.id} ${item.title} ${item.instructions} ${item.cron} ${item.workspace?.path ?? ""}`
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
  );
  return (
    <ViewPanel
      view={view}
      title="Schedules"
      description="Saved schedules. Use the controls above to add, edit or run a schedule."
    >
      <Toolbar label="Schedule filters">
        <SearchField
          id="search"
          aria-label="Search this page"
          placeholder="Title or instructions"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <Select
          id="state"
          label="Schedule status"
          value={status}
          onValueChange={setStatus}
          options={[
            { value: "all", label: "All statuses" },
            { value: "enabled", label: "Enabled" },
            { value: "disabled", label: "Disabled" },
          ]}
        />
      </Toolbar>
      <Metadata>
        <span id="summary">{`${rows.filter((item) => item.enabled).length} enabled · ${rows.filter((item) => !item.enabled).length} disabled on this page`}</span>
      </Metadata>
      <ReadStatus
        view={view}
        ready={`${rows.length} schedules loaded on this page.`}
        loading="Reading schedules…"
      />
      <RecordGroup id="items" aria-label="Saved schedules">
        {!matching.length && (
          <EmptyState
            icon={CalendarClock}
            size="compact"
            title={
              rows.length
                ? "No matching schedules on this page."
                : "No schedules saved."
            }
            sub={
              rows.length
                ? "Clear the search or select All statuses."
                : "Use Add schedule above to save recurring work."
            }
          />
        )}
        {matching.map((item) => (
          <ListRow key={item.id} data-schedule-id={item.id}>
            <Toolbar label={item.title || "Untitled schedule"}>
              <RecordTitle>{item.title || "Untitled schedule"}</RecordTitle>
              <Badge tone={item.enabled ? "success" : "neutral"}>
                {item.enabled ? "Enabled" : "Disabled"}
              </Badge>
            </Toolbar>
            <RecordBody>{item.instructions}</RecordBody>
            <Facts
              values={[
                ["Schedule", item.id],
                ["Revision", item.revision],
                ["Cron", item.cron],
                ["Workspace", item.workspace?.path ?? "Runtime default"],
                [
                  "Model",
                  item.provider && item.model
                    ? `${item.provider}/${item.model}`
                    : "Runtime default",
                ],
                ["Reasoning effort", item.reasoningEffort],
                ["Next firing", item.nextRunAt ?? "Not scheduled"],
                ["Last firing", item.lastRunAt ?? "Never fired"],
                ["Created", item.createdAt],
              ]}
            />
          </ListRow>
        ))}
      </RecordGroup>
      <Pagination
        view={view}
        nextCursor={page.nextCursor}
        label="Schedule pages"
      />
    </ViewPanel>
  );
}
mountView(Schedules);
