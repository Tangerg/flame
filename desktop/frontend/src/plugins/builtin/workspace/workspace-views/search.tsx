import * as stylex from "@stylexjs/stylex";
import { useRef, useState } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import {
  DataView,
  EmptyState,
  Icon,
  Pressable,
  SearchField,
  TextButton,
  chevron,
  vocab,
} from "@/ui";
import { useT } from "@/lib/i18n";
import { color, face, radius, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./views/viewStyles";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceGrep } from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  WORKSPACE_SEARCH_MATCH_LIMIT,
  workspaceSearchSubtext,
  workspaceSearchViewModel,
} from "@/plugins/builtin/workspace/application/searchViewModel";
import { openWorkspaceFile } from "@/plugins/builtin/workspace/public/navigation";
import {
  rememberWorkspaceView,
  useWorkspaceViewMemory,
} from "@/plugins/builtin/workspace/application/navigation";

const sx = stylex.create({
  fields: { display: "grid", gridTemplateColumns: "minmax(0, 3fr) minmax(0, 2fr)", gap: space.s2 },
  hit: { borderRadius: radius.step2xs, backgroundColor: surface.accentWash, color: color.fg },
  groupHead: {
    display: "flex",
    width: "100%",
    alignItems: "center",
    gap: space.s1_5,
    borderWidth: 0,
    backgroundColor: "transparent",
    padding: 0,
    textAlign: "left",
  },
});

const LEAD_CHARS = 32;

function Snippet({ text, query }: { text: string; query: string }) {
  const at = query ? text.toLocaleLowerCase().indexOf(query.toLocaleLowerCase()) : -1;
  if (at < 0) return <>{text}</>;
  const start = Math.max(0, at - LEAD_CHARS);
  return (
    <>
      {start > 0 && "…"}
      {text.slice(start, at)}
      <span data-slot="search-hit" {...stylex.props(sx.hit)}>
        {text.slice(at, at + query.length)}
      </span>
      {text.slice(at + query.length)}
    </>
  );
}

export function SearchTab() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const memory = useWorkspaceViewMemory();
  const scopeField = useRef<HTMLInputElement>(null);
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set());
  const [query] = useDebouncedValue(memory.searchQuery.trim(), { wait: 300 });
  const [scope] = useDebouncedValue(memory.searchPath.trim(), { wait: 300 });
  const { data, isLoading, error, refetch } = useWorkspaceGrep(
    query && workspace.status === "ready"
      ? {
          query,
          cwd: workspace.cwd,
          limit: WORKSPACE_SEARCH_MATCH_LIMIT,
          ...(scope ? { path: scope } : {}),
        }
      : undefined,
  );
  const view = workspaceSearchViewModel(data);
  const toggleGroup = (path: string) =>
    setCollapsed((previous) => {
      const next = new Set(previous);
      if (!next.delete(path)) next.add(path);
      return next;
    });

  return (
    <WorkspaceViewLayout icon="search" title="search.title" sub={workspaceSearchSubtext(t, view)}>
      <div {...stylex.props(vs.gutter, vs.statusPad, sx.fields)}>
        <SearchField
          font="mono"
          value={memory.searchQuery}
          onValueChange={(searchQuery) => rememberWorkspaceView({ searchQuery })}
          placeholder={t("search.placeholder")}
          aria-label={t("search.aria")}
          spellCheck={false}
        />
        <SearchField
          ref={scopeField}
          font="mono"
          value={memory.searchPath}
          onValueChange={(searchPath) => rememberWorkspaceView({ searchPath })}
          placeholder={t("search.scope.placeholder")}
          aria-label={t("search.scope.aria")}
          spellCheck={false}
        />
      </div>
      {query === "" ? (
        <EmptyState
          icon="search"
          size="compact"
          title={t("search.prompt.title")}
          sub={t("search.scope")}
        />
      ) : (
        <DataView
          items={data ? view.groups : undefined}
          isLoading={isLoading || workspace.status === "resolving"}
          failure={error}
          onRetry={refetch}
          skeletonCount={4}
          empty={{
            icon: "search",
            title: t("search.empty.title"),
            sub: t("search.empty.sub"),
            size: "compact",
          }}
        >
          {(groups) => (
            <div {...stylex.props(vocab.column, vs.padBottom)}>
              {groups.map((group) => {
                const shut = collapsed.has(group.path);
                return (
                  <div key={group.path} {...stylex.props(vs.gutter, vs.groupPad)}>
                    <Pressable
                      type="button"
                      aria-expanded={!shut}
                      title={group.path}
                      onClick={() => toggleGroup(group.path)}
                      className={
                        stylex.props(sx.groupHead, vs.title, typeStep.uiMd, face.mono).className
                      }
                    >
                      <Icon
                        name="chevron-down"
                        size="xs"
                        className={stylex.props(chevron.base, shut && chevron.shut).className}
                      />
                      <span {...stylex.props(vocab.truncate)}>{group.path}</span>
                      <span {...stylex.props(vs.matchCount)}>{group.matchCount}</span>
                    </Pressable>
                    {!shut && (
                      <div {...stylex.props(vocab.column, vs.afterTitle)}>
                        {group.matches.map((m) => (
                          <Pressable
                            key={m.lineNumber}
                            onClick={() => openWorkspaceFile(group.path, m.lineNumber)}
                            className={stylex.props(vs.matchRow, vs.wash, typeStep.uiMd).className}
                          >
                            <span {...stylex.props(vs.lineNumber, typeStep.uiSm)}>
                              {m.lineNumber}
                            </span>
                            <span {...stylex.props(vocab.truncate, vocab.soft)} title={m.text}>
                              <Snippet text={m.text} query={query} />
                            </span>
                          </Pressable>
                        ))}
                      </div>
                    )}
                  </div>
                );
              })}
              {view.overflowCount > 0 && (
                <div
                  {...stylex.props(vs.gutter, vs.rowPad, vocab.line, vocab.faint, typeStep.uiSm)}
                >
                  <span>… {t("search.overflow", { count: view.overflowCount })}</span>
                  <TextButton tone="accent" size="sm" onClick={() => scopeField.current?.focus()}>
                    {t("search.narrow")}
                  </TextButton>
                </div>
              )}
            </div>
          )}
        </DataView>
      )}
    </WorkspaceViewLayout>
  );
}
