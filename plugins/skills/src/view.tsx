import { useState } from "react";
import { BookOpen } from "@flame/icons/react";
import type {
  ManagedSkill,
  Page,
  Skill,
  SkillDetail,
  SkillDiscovery,
  SkillProposal,
} from "@flame/runtime-contract/wire";
import {
  CodeBlock,
  Disclosure,
  EmptyState,
  Facts,
  ListRow,
  Metadata,
  RecordBody,
  RecordGroup,
  RecordTitle,
  SearchField,
  Toolbar,
} from "@flame/ui";
import {
  mountView,
  ReadStatus,
  useRead,
  ViewPanel,
  type ViewProps,
} from "../../src/view";

type SkillSnapshot =
  | { type: "available"; data: SkillDiscovery }
  | { type: "detail"; data: SkillDetail }
  | { type: "library"; data: Page<ManagedSkill> }
  | { type: "review"; data: Page<SkillProposal> };
type SkillRecord = Skill | SkillDetail | ManagedSkill | SkillProposal;
function SkillRow({ item, review }: { item: SkillRecord; review: boolean }) {
  return (
    <ListRow>
      <RecordTitle>{item.name}</RecordTitle>
      {item.description && <RecordBody>{item.description}</RecordBody>}
      <Facts
        values={[
          ["Scope", "scope" in item ? item.scope : undefined],
          ["Lifecycle", "lifecycle" in item ? item.lifecycle : undefined],
          ["Revision", "revision" in item ? item.revision : undefined],
          ["Source path", "path" in item ? item.path : undefined],
          [
            "Installation",
            "installation" in item
              ? item.installation?.installationId
              : undefined,
          ],
          [
            "Release",
            "installation" in item ? item.installation?.digest : undefined,
          ],
          ["Origin", "origin" in item ? item.origin : undefined],
          [
            "Source Session",
            "sourceSession" in item ? item.sourceSession : undefined,
          ],
          [
            "Revises an existing Skill",
            review
              ? String("revises" in item && item.revises === true)
              : undefined,
          ],
        ]}
      />
      {"instructions" in item && (
        <Disclosure title="Read instructions">
          <CodeBlock label={`Instructions for ${item.name}`}>
            {item.instructions}
          </CodeBlock>
        </Disclosure>
      )}
    </ListRow>
  );
}
function Skills({ view }: ViewProps<SkillSnapshot>) {
  const { data: snapshot } = useRead(view);
  const [search, setSearch] = useState("");
  const titles = {
    available: "Available Skills",
    detail: "Skill document",
    library: "Personal Skill library",
    review: "Skill proposals",
  };
  const rows: SkillRecord[] =
    snapshot.type === "available"
      ? snapshot.data.skills
      : snapshot.type === "detail"
        ? [snapshot.data]
        : snapshot.data.data;
  const matching = rows.filter((item) =>
    `${item.name} ${item.description ?? ""} ${"scope" in item ? item.scope : "lifecycle" in item ? item.lifecycle : ""}`
      .toLocaleLowerCase()
      .includes(search.toLocaleLowerCase()),
  );
  return (
    <ViewPanel
      view={view}
      title={titles[snapshot.type]}
      description="Inspect Skills and proposals. Use the controls above to manage your personal library and review publication."
    >
      <Toolbar label="Skill filters">
        <SearchField
          id="search"
          aria-label="Search this catalog"
          placeholder="Name or description"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </Toolbar>
      <Metadata>
        <span id="summary">
          {snapshot.type === "detail"
            ? "Resolved document"
            : `${rows.length} ${snapshot.type === "review" ? "pending proposals" : snapshot.type === "library" ? "user-managed Skills" : "available Skills"}`}
        </span>
      </Metadata>
      {snapshot.type === "available" &&
        snapshot.data.diagnostics.length > 0 && (
          <RecordBody>
            {snapshot.data.diagnostics
              .map((item) => `${item.name}: ${item.detail}`)
              .join("\n")}
          </RecordBody>
        )}
      <ReadStatus
        view={view}
        ready="Skill read completed."
        loading="Reading Skills…"
      />
      <RecordGroup id="items" aria-label={titles[snapshot.type]}>
        {matching.map((item) => (
          <SkillRow
            key={item.name}
            item={item}
            review={snapshot.type === "review"}
          />
        ))}
        {!matching.length && (
          <EmptyState
            icon={BookOpen}
            size="compact"
            title={
              rows.length
                ? "No matching Skills in this catalog."
                : snapshot.type === "review"
                  ? "No proposals pending review."
                  : snapshot.type === "library"
                    ? "No managed Skills in this library."
                    : "No Skills offered in this workspace."
            }
            sub={
              rows.length
                ? "Clear the search to show Skills."
                : "Select another catalog above or refresh after adding a Skill."
            }
          />
        )}
      </RecordGroup>
    </ViewPanel>
  );
}
mountView(Skills);
