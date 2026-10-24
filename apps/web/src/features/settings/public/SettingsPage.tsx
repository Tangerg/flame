import * as stylex from "@stylexjs/stylex";
import { type ReactNode, type Ref } from "react";
import { Suspense, useRef, useState } from "react";
import { knownIconName } from "@/ui/index";
import { SkeletonList } from "@/ui/index";
import { TextButton } from "@/ui/index";
import { VerticalTabs } from "@/ui/index";
import { HeadingLevel, isImeKey, Panel, PanelHeader, SearchField } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { PluginBoundary } from "@/extensions/host/PluginBoundary";
import {
  openWorkspaceSettingsPane,
  useWorkspaceSettingsPaneTarget,
} from "@/features/workspace/public/navigation";
import { useSettingsPanes } from "@/features/settings/public/panes";
import { color, layout, space, type as typeStep } from "@flame/ui/tokens.stylex";
const sp = stylex.create({
  body: { display: "flex", flexDirection: "column", gap: layout.sectionGap, minWidth: 0 },
  search: { padding: space.s4 },
  empty: {
    display: "flex",
    flexDirection: "column",
    alignItems: "flex-start",
    gap: space.s1_5,
    paddingInline: space.s2,
    paddingBlock: space.s3,
    color: color.fgMuted,
  },
});

const GROUPS: { id: string; labelKey: string }[] = [
  { id: "general", labelKey: "settings.group.general" },
  { id: "models", labelKey: "settings.group.models" },
  { id: "agent", labelKey: "settings.group.agent" },
  { id: "integrations", labelKey: "settings.group.integrations" },
  { id: "advanced", labelKey: "settings.group.advanced" },
];

const FALLBACK_GROUP = "advanced";

export function SettingsPage() {
  const t = useT();
  const panes = useSettingsPanes();
  const targetPane = useWorkspaceSettingsPaneTarget();
  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const normalizedQuery = query.trim().toLocaleLowerCase();

  const known = new Set(GROUPS.map((g) => g.id));
  const grouped = GROUPS.map((g) => ({
    ...g,
    label: t(g.labelKey),
    items: panes
      .filter((p) => (p.group && known.has(p.group) ? p.group : FALLBACK_GROUP) === g.id)
      .map((p) => ({
        id: p.id,
        label: t(p.label),
        icon: knownIconName(p.icon),
        terms: [p.label, p.description, ...(p.keywords ?? [])]
          .filter((key): key is string => key !== undefined)
          .map((key) => t(key).toLocaleLowerCase()),
        content: (
          <SettingsPaneFrame
            title={t(p.label)}
            description={p.description ? t(p.description) : undefined}
          >
            <PluginBoundary plugin={`settings:${p.id}`} label={t(p.label)}>
              <Suspense fallback={<SkeletonList count={4} label={t("common.loading")} />}>
                <p.component />
              </Suspense>
            </PluginBoundary>
          </SettingsPaneFrame>
        ),
      })),
  })).filter((g) => g.items.length > 0);
  const allItems = grouped.flatMap((group) => group.items);
  const matches = (item: { id: string }) =>
    !normalizedQuery ||
    (allItems.find((candidate) => candidate.id === item.id)?.terms ?? []).some((term) =>
      term.includes(normalizedQuery),
    );
  const firstMatch = allItems.find(matches)?.id;
  const activeId =
    targetPane && allItems.some((p) => p.id === targetPane) ? targetPane : allItems[0]?.id;

  return (
    <VerticalTabs
      ariaLabel={t("settings.title")}
      groups={grouped}
      value={activeId}
      onValueChange={(pane) => {
        if (pane) openWorkspaceSettingsPane(pane);
      }}
      railFilter={matches}
      railEmpty={
        <div role="status" {...stylex.props(sp.empty, typeStep.uiSm)}>
          <span>{t("settings.search.empty", { query: query.trim() })}</span>
          <TextButton
            tone="accent"
            onClick={() => {
              setQuery("");
              searchRef.current?.focus();
            }}
          >
            {t("common.clearSearch")}
          </TextButton>
        </div>
      }
      railHeader={
        <SettingsRailHeader
          ref={searchRef}
          query={query}
          onQueryChange={setQuery}
          onSubmit={() => {
            if (firstMatch) openWorkspaceSettingsPane(firstMatch);
          }}
          searchPlaceholder={t("settings.searchPlaceholder")}
        />
      }
    />
  );
}

function SettingsPaneFrame({
  title,
  description,
  children,
}: {
  title: string;
  description?: ReactNode;
  children: ReactNode;
}) {
  return (
    <HeadingLevel value={1}>
      <Panel label={title} header={<PanelHeader title={title} description={description} />}>
        <div {...stylex.props(sp.body)}>{children}</div>
      </Panel>
    </HeadingLevel>
  );
}

function SettingsRailHeader({
  ref,
  query,
  onQueryChange,
  onSubmit,
  searchPlaceholder,
}: {
  ref: Ref<HTMLInputElement>;
  query: string;
  onQueryChange: (value: string) => void;
  onSubmit: () => void;
  searchPlaceholder: string;
}) {
  const t = useT();
  return (
    <div {...stylex.props(sp.search)}>
      <SearchField
        ref={ref}
        size="lg"
        value={query}
        onValueChange={onQueryChange}
        onClear={() => onQueryChange("")}
        clearLabel={t("common.clearSearch")}
        onKeyDown={(event) => {
          if (event.key === "Enter" && !isImeKey(event.nativeEvent)) onSubmit();
        }}
        placeholder={searchPlaceholder}
        aria-label={searchPlaceholder}
      />
    </div>
  );
}
