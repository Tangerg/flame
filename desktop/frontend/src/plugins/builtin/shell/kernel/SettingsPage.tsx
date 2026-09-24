import { isImeKey } from "@/lib/ime";
import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { Suspense, useState } from "react";
import {
  Button,
  Icon,
  knownIconName,
  SearchField,
  SkeletonList,
  TextButton,
  VerticalTabs,
  vocab,
} from "@/ui";
import { AgentSurfaceHeader } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import {
  openWorkspaceSettingsPane,
  selectWorkspaceChat,
  useWorkspaceSettingsPaneTarget,
} from "@/plugins/builtin/workspace/public/navigation";
import { useSettingsPanes } from "@/plugins/sdk";
import { color, space, type as typeStep, weight } from "@/styles/tokens.stylex";

const sp = stylex.create({
  title: { color: color.fg, fontWeight: weight.semibold },
  blurb: {
    marginTop: space.s1_5,
    maxWidth: "60ch",
    lineHeight: "1.5rem",
    color: color.fgMuted,
  },
  body: { marginTop: space.s6, paddingBottom: space.s12 },
  backRow: { paddingInline: space.s4, paddingBottom: space.s4 },
  back: { marginBottom: space.s3, alignSelf: "flex-start" },
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
            <PluginBoundary plugin={`settings:${p.id}`}>
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
    targetPane && allItems.some((p) => p.id === targetPane)
      ? targetPane
      : (firstMatch ?? allItems[0]?.id);

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
          <TextButton tone="accent" onClick={() => setQuery("")}>
            {t("settings.search.clear")}
          </TextButton>
        </div>
      }
      railHeader={
        <SettingsRailHeader
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
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section>
      <header>
        <h1 {...stylex.props(sp.title, typeStep.displaySm)}>{title}</h1>
        {description && <p {...stylex.props(sp.blurb, typeStep.uiMd)}>{description}</p>}
      </header>
      <div {...stylex.props(sp.body)}>{children}</div>
    </section>
  );
}

function SettingsRailHeader({
  query,
  onQueryChange,
  onSubmit,
  searchPlaceholder,
}: {
  query: string;
  onQueryChange: (value: string) => void;
  onSubmit: () => void;
  searchPlaceholder: string;
}) {
  const t = useT();
  return (
    <div {...stylex.props(vocab.column)}>
      <AgentSurfaceHeader divider={false} corner="window" aria-hidden />
      <div {...stylex.props(sp.backRow)}>
        <Button
          type="button"
          variant="ghost"
          size="md"
          press="none"
          data-chrome-focus=""
          onClick={selectWorkspaceChat}
          className={stylex.props(sp.back).className}
        >
          <Icon name="arrow-left" size="md" full />
          <span>{t("settings.backToApp")}</span>
        </Button>
        <SearchField
          size="lg"
          value={query}
          onValueChange={onQueryChange}
          onClear={() => onQueryChange("")}
          onKeyDown={(event) => {
            if (event.key === "Escape" && query) {
              event.preventDefault();
              onQueryChange("");
            }
            if (event.key === "Enter" && !isImeKey(event.nativeEvent)) onSubmit();
          }}
          placeholder={searchPlaceholder}
          aria-label={searchPlaceholder}
        />
      </div>
    </div>
  );
}
