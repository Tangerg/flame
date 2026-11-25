import { BookOpen, Refresh } from "@flame/icons/react";
import { type Translate } from "@flame/component-sdk";
import {
  IconButton,
  EmptyState,
  SearchField,
  FilterBar,
  Hang,
  Segmented,
  Toolbar,
} from "@flame/ui";
import { type SkillSnapshot, type Target } from "./types";
import { SECTION_VALUES, type SkillsState } from "./useSkills";

export function catalogSummary(
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

export function SkillCatalogControls({
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
  searchField: SkillsState["search"]["field"];
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

export function catalogDescription(
  t: Translate,
  target: Target,
  workspace: unknown,
): string | false {
  // The personal library is read without a workspace.
  return target !== "library" && !workspace && t("section.selectWorkspace");
}

export function SkillEmptyState({
  t,
  type,
  filtered,
}: {
  t: Translate;
  type: Target;
  filtered: boolean;
}) {
  const review = type === "review";
  const state = filtered ? "filtered" : "none";
  return (
    <EmptyState
      icon={BookOpen}
      size="compact"
      title={t(review ? `empty.review.${state}.title` : `empty.${state}.title`)}
    />
  );
}
