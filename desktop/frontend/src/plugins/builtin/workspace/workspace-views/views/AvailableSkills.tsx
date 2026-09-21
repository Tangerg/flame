import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { Collapsible, DataView, FilePath, SystemMessage, Tag, TextButton, Well, vocab } from "@/ui";
import { type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./viewStyles";
import { useT } from "@/lib/i18n";
import {
  useWorkspaceSkills,
  useWorkspaceSkillDetail,
} from "@/plugins/builtin/workspace/application/workspaceQueries";
import { useWorkspaceCapability } from "@/plugins/builtin/workspace/application/workspaceCapabilities";
import { workspaceSkillsViewModel } from "@/plugins/builtin/workspace/application/workspaceCatalogViewModel";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";

export function AvailableSkills() {
  const t = useT();
  const skillsEnabled = useWorkspaceCapability("skills");
  const workspace = useActiveSessionWorkspace();
  const { data, isLoading, error, refetch } = useWorkspaceSkills(
    workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
  const view = workspaceSkillsViewModel(data?.skills ?? [], skillsEnabled);

  return (
    <>
      {view.enabled && (
        <div {...stylex.props(vs.gutter, vs.rowPad, typeStep.uiSm, vocab.muted)}>
          {t("skills.available", { count: view.count })}
        </div>
      )}
      {view.enabled && data && data.diagnostics.length > 0 && (
        <div {...stylex.props(vs.gutter, vs.rowPad)}>
          <SystemMessage variant="warning" shape="form">
            <div {...stylex.props(vocab.column)}>
              <div>{t("skills.unreadable", { count: data.diagnostics.length })}</div>
              {data.diagnostics.map((diagnostic) => (
                <div key={diagnostic.name} {...stylex.props(typeStep.uiSm)}>
                  <span {...stylex.props(vs.title)}>{diagnostic.name}</span> — {diagnostic.detail}
                </div>
              ))}
            </div>
          </SystemMessage>
        </div>
      )}
      <DataView
        items={view.rows}
        isLoading={view.enabled && (isLoading || workspace.status === "resolving")}
        failure={view.enabled ? error : undefined}
        onRetry={refetch}
        skeletonCount={4}
        empty={
          skillsEnabled
            ? {
                icon: "sparkle",
                title: t("skills.empty.title"),
                sub: t("skills.empty.sub"),
              }
            : {
                icon: "sparkle",
                title: t("skills.disabled.title"),
                sub: t("skills.disabled.sub"),
              }
        }
      >
        {(rows) => (
          <div {...stylex.props(vocab.column)}>
            {rows.map((s) => (
              <div key={s.id} {...stylex.props(vs.gutter, vs.rowPad)}>
                <div {...stylex.props(vocab.line, vocab.min)}>
                  <div {...stylex.props(vs.title, vocab.truncate, typeStep.uiMd)}>{s.name}</div>
                  {s.scope && <Tag>{s.scope}</Tag>}
                </div>
                {s.description && (
                  <div {...stylex.props(vs.description, typeStep.uiSm)}>{s.description}</div>
                )}
                {workspace.status === "ready" && (
                  <SkillInspection
                    key={`${workspace.cwd ?? ""} ${s.name}`}
                    cwd={workspace.cwd}
                    name={s.name}
                  />
                )}
              </div>
            ))}
          </div>
        )}
      </DataView>
    </>
  );
}

function SkillInspection({ cwd, name }: { cwd?: string; name: string }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <>
      <TextButton
        size="sm"
        className={stylex.props(vocab.afterLine).className}
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        {open ? t("skillProposals.hideBody") : t("skillProposals.readBody")}
      </TextButton>
      {/* Mounted only once asked: the body is a whole document per skill, and a list of them
          would fetch every one to render a row nobody opened. */}
      {open && (
        <Collapsible open>
          <SkillDetail cwd={cwd} name={name} />
        </Collapsible>
      )}
    </>
  );
}

function SkillDetail({ cwd, name }: { cwd?: string; name: string }) {
  const t = useT();
  const { data, isLoading, error, refetch } = useWorkspaceSkillDetail({ cwd, name });
  return (
    <DataView
      items={data ? [data] : []}
      isLoading={isLoading}
      failure={error}
      onRetry={refetch}
      skeletonCount={1}
      empty={{ icon: "file", title: t("skills.body.empty.title"), sub: t("skills.body.empty.sub") }}
    >
      {(rows) =>
        rows.map((detail) => (
          <div key={detail.revision} {...stylex.props(vocab.column, vocab.afterLine)}>
            <div {...stylex.props(vocab.line, vocab.min)}>
              <FilePath path={detail.path} />
              <Tag title={t("skills.revision", { revision: detail.revision })}>
                {detail.revision.slice(0, 7)}
              </Tag>
            </div>
            <Well>{detail.instructions}</Well>
          </div>
        ))
      }
    </DataView>
  );
}
