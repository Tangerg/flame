import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { DataView, Pressable, SearchField } from "@/ui";
import { useT } from "@/lib/i18n";
import { face, type as typeStep } from "@/styles/tokens.stylex";
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

export function SearchTab() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const [input, setInput] = useState("");
  const [query] = useDebouncedValue(input.trim(), { wait: 300 });
  const { data, isLoading, isError, refetch } = useWorkspaceGrep(
    query && workspace.status === "ready"
      ? { query, cwd: workspace.cwd, limit: WORKSPACE_SEARCH_MATCH_LIMIT }
      : undefined,
  );
  const view = workspaceSearchViewModel(data);

  return (
    <WorkspaceViewLayout
      icon="search"
      titleStrong
      title="search.title"
      sub={workspaceSearchSubtext(t, view) ?? t("search.noMatches")}
      scrollClassName="py-1"
    >
      <div {...stylex.props(vs.gutter, vs.statusPad)}>
        <SearchField
          font="mono"
          value={input}
          onValueChange={setInput}
          placeholder={t("search.placeholder")}
          aria-label={t("search.aria")}
          spellCheck={false}
        />
      </div>
      {query === "" ? null : (
        <DataView
          items={data ? view.groups : undefined}
          isLoading={isLoading || workspace.status === "resolving"}
          isError={isError}
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
            <div {...stylex.props(vs.stack, vs.padBottom)}>
              {groups.map((group) => (
                <div key={group.path} {...stylex.props(vs.gutter, vs.groupPad)}>
                  <div {...stylex.props(vs.title, vs.truncate, typeStep.uiSm, face.mono)}>
                    {group.path}
                    <span {...stylex.props(vs.matchCount)}>{group.matchCount}</span>
                  </div>
                  <div {...stylex.props(vs.stack, vs.afterTitle)}>
                    {group.matches.map((m) => (
                      <Pressable
                        key={m.lineNumber}
                        onClick={() => openWorkspaceFile(group.path, m.lineNumber)}
                        className={stylex.props(vs.matchRow, vs.wash, typeStep.uiMd).className}
                      >
                        <span {...stylex.props(vs.lineNumber, typeStep.uiSm)}>{m.lineNumber}</span>
                        <span {...stylex.props(vs.truncate, vs.soft)} title={m.text}>
                          {m.text}
                        </span>
                      </Pressable>
                    ))}
                  </div>
                </div>
              ))}
              {view.overflowCount > 0 && (
                <div {...stylex.props(vs.gutter, vs.rowPad, vs.caption, typeStep.uiSm)}>
                  … {t("search.overflow", { count: view.overflowCount })}
                </div>
              )}
            </div>
          )}
        </DataView>
      )}
    </WorkspaceViewLayout>
  );
}
