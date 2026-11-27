import { useEffect, useState } from "react";
import {
  failureText,
  mutationSettlementIsUnknown,
  type bindOperations,
  type ComponentContext,
} from "@flame/component-sdk";
import { projectNumber } from "@frame/sdk/json";
import type { MutationPromise } from "@flame/runtime-client";
import {
  Button,
  Checkbox,
  FormActions,
  FormField,
  FormRow,
  Metadata,
  PanelSection,
  Select,
  TextArea,
  TextField,
} from "@flame/ui";
import type {
  MethodInputs,
  MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";

const listWork = "flame.knowledgebase/list-work.v1";
const readWork = "flame.knowledgebase/read-work.v1";
const acceptWork = "flame.knowledgebase/accept-work.v1";
const dependency = "research-artifacts";
type Work = MethodOutputs[typeof readWork];
type Receipt = MethodOutputs[typeof acceptWork];
type Operations = ReturnType<
  typeof bindOperations<MethodInputs, MethodOutputs>
>;
type Claim = MethodOutputs["flame.knowledgebase/list-claims.v1"][number];

const english = {
  title: "Research deliverables",
  refresh: "Revalidate",
  current: "Accepted",
  recheck: "Needs revalidation",
  method: "Method name",
  instructions: "Reusable procedure",
  name: "Work title",
  key: "Work ID",
  claims: "Accepted claims",
  provider: "Deliverables installation",
  artifact: "Native document ID",
  revision: "Native revision ID",
  chart: "Chart element ID (optional)",
  accept: "Accept work",
  edit: "Update acceptance",
  reuse: "Reuse the method",
  pinned: "Open reviewed revision",
  latest: "Open latest editable deliverable",
  retry: "Check original command",
  error: "Unable to read or save research work.",
};
const chinese: typeof english = {
  title: "研究交付",
  refresh: "重新校验",
  current: "已验收",
  recheck: "待重新验收",
  method: "方法名称",
  instructions: "可复用步骤",
  name: "工作标题",
  key: "工作 ID",
  claims: "已接受的论断",
  provider: "成果交付安装",
  artifact: "原生文档 ID",
  revision: "原生版本 ID",
  chart: "图表元素 ID（可选）",
  accept: "验收工作",
  edit: "更新验收",
  reuse: "复用此方法",
  pinned: "打开已验收版本",
  latest: "打开最新可编辑交付",
  retry: "查询原命令",
  error: "无法读取或保存研究工作。",
};

const empty = {
  id: "",
  expectedRevision: 0,
  title: "",
  method: { name: "", instructions: "" },
  artifact: { installationId: "", id: "", revision: "", object: "" },
  claims: [] as string[],
};

export function WorkReview({
  operations,
  context,
  locale,
  project,
  claims,
  refresh,
}: {
  operations: Operations;
  context: ComponentContext;
  locale: string;
  project: string;
  claims: Claim[];
  refresh: number;
}) {
  const t = locale.startsWith("zh") ? chinese : english;
  const [items, setItems] = useState<Receipt[]>([]);
  const [id, setID] = useState("");
  const [visible, setVisible] = useState<Work>();
  const [providers, setProviders] = useState<string[]>([]);
  const [draft, setDraft] = useState(empty);
  const [error, setError] = useState<string>();
  const [reload, setReload] = useState(0);
  const [busy, setBusy] = useState(false);
  const [recover, setRecover] = useState<MutationPromise<Receipt>>();
  useEffect(() => {
    const owner = context.binding;
    if (!owner) return;
    const reader = new AbortController();
    void context
      .track((signal) =>
        context.runtime.plugins.resolveRequirements(
          {
            installationId: owner.installationId,
            digest: owner.digest,
            contribution: owner.contribution,
          },
          AbortSignal.any([reader.signal, signal]),
        ),
      )
      .then(
        (result) => {
          if (!reader.signal.aborted)
            setProviders(
              result.data
                .find((entry) => entry.id === dependency)
                ?.bindings.map((binding) => binding.installationId) ?? [],
            );
        },
        (cause) => {
          if (!reader.signal.aborted) setError(failureText(cause) ?? t.error);
        },
      );
    return () => reader.abort();
  }, [context, refresh, reload, t.error]);
  useEffect(() => {
    const reader = new AbortController();
    setVisible(undefined);
    void context
      .track((signal) => {
        const scope = AbortSignal.any([reader.signal, signal]);
        return Promise.all([
          operations.query(listWork, { project }, scope),
          id
            ? operations.query(readWork, { project, id }, scope)
            : Promise.resolve(undefined),
        ]);
      })
      .then(
        ([rows, work]) => {
          if (!reader.signal.aborted) {
            setItems(rows);
            setVisible(work);
          }
        },
        (cause) => {
          if (!reader.signal.aborted) setError(failureText(cause) ?? t.error);
        },
      );
    return () => reader.abort();
  }, [context, id, operations, project, refresh, reload, t.error]);

  const chosen = claims.filter(
    (claim) => claim.state === "current" && draft.claims.includes(claim.id),
  );
  async function commit(command: MutationPromise<Receipt>) {
    setBusy(true);
    setError(undefined);
    try {
      const original = await command;
      setID(original.id);
      setDraft(empty);
      setRecover(undefined);
      setReload((value) => value + 1);
    } catch (cause) {
      setError(failureText(cause) ?? t.error);
      setRecover(mutationSettlementIsUnknown(cause) ? command : undefined);
    } finally {
      setBusy(false);
    }
  }
  function openArtifact(work: Work, latest: boolean, origin: HTMLElement) {
    if (!work.artifact || !work.evidence) return;
    const { id: document, revision, object, installationId } = work.artifact;
    const target = JSON.stringify({
      document,
      revision: latest ? work.evidence.head : revision,
      object: latest && !work.evidence.objectAtHead ? undefined : object,
    });
    void context.views
      ?.openRelated?.(
        dependency,
        installationId,
        "deliverables",
        target,
        origin,
      )
      .catch((cause) => setError(failureText(cause) ?? t.error));
  }
  function editAcceptance(work: Work) {
    if (!work.artifact || !work.method || !work.evidence) return;
    setDraft({
      id: work.id,
      title: work.title,
      expectedRevision: projectNumber(work.revision),
      method: work.method,
      claims: work.claims.map((claim) => claim.id),
      artifact: {
        ...work.artifact,
        revision: work.evidence.head ?? work.artifact.revision,
        object: work.evidence.objectAtHead ? (work.artifact.object ?? "") : "",
      },
    });
  }

  return (
    <PanelSection title={t.title}>
      {error && <p role="alert">{error}</p>}
      <FormActions label={t.title}>
        {items.map((item) => (
          <Button
            key={item.id}
            size="sm"
            variant={item.id === id ? "primary" : "ghost"}
            onClick={() => setID(item.id)}
          >
            {item.id}
          </Button>
        ))}
        <Button
          size="sm"
          onClick={() => {
            setError(undefined);
            setReload((value) => value + 1);
          }}
        >
          {t.refresh}
        </Button>
      </FormActions>
      {visible && (
        <section>
          <p>
            {visible.title || visible.id} ·{" "}
            {visible.state === "current" ? t.current : t.recheck}
          </p>
          <Metadata>
            {visible.method?.name} · {visible.methodVersion}
          </Metadata>
          {visible.claims.map((claim) => (
            <div key={claim.id}>
              <p>
                {claim.statement || claim.id} · {claim.state}
              </p>
              {claim.evidence.map((item) => (
                <Button
                  key={JSON.stringify(item)}
                  size="sm"
                  variant="ghost"
                  onClick={(event) =>
                    context.views?.open(
                      "project-research",
                      JSON.stringify({
                        project,
                        material: item.documentId,
                        revision: projectNumber(item.revision),
                        page: projectNumber(item.page),
                        firstLine: projectNumber(item.firstLine),
                        lastLine: projectNumber(item.lastLine),
                      }),
                      event.currentTarget,
                    )
                  }
                >
                  {item.title} · {String(item.page)} · {String(item.firstLine)}–
                  {String(item.lastLine)}
                </Button>
              ))}
            </div>
          ))}
          {visible.artifact && (
            <FormActions label={t.title}>
              <Button
                size="sm"
                onClick={(event) =>
                  openArtifact(visible, false, event.currentTarget)
                }
              >
                {t.pinned}
              </Button>
              <Button
                size="sm"
                onClick={(event) =>
                  openArtifact(visible, true, event.currentTarget)
                }
              >
                {t.latest}
              </Button>
              <Button
                size="sm"
                disabled={busy || !!recover}
                onClick={() => editAcceptance(visible)}
              >
                {t.edit}
              </Button>
            </FormActions>
          )}
          {visible.method && (
            <Button
              size="sm"
              disabled={busy || !!recover}
              onClick={() => setDraft({ ...empty, method: visible.method! })}
            >
              {t.reuse}
            </Button>
          )}
        </section>
      )}
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (busy || recover || chosen.length === 0) return;
          void commit(
            operations.command(acceptWork, {
              project,
              id: draft.id.trim(),
              title: draft.title.trim(),
              expectedRevision: draft.expectedRevision,
              method: {
                name: draft.method.name.trim(),
                instructions: draft.method.instructions.trim(),
              },
              claims: chosen.map((claim) => ({
                id: claim.id,
                revision: claim.revision,
              })),
              artifact: {
                ...draft.artifact,
                installationId:
                  draft.artifact.installationId || providers[0] || "",
                id: draft.artifact.id.trim(),
                revision: draft.artifact.revision.trim(),
                object: draft.artifact.object.trim() || undefined,
              },
            }),
          );
        }}
      >
        <fieldset disabled={busy || !!recover}>
          <FormRow>
            <FormField label={t.key}>
              <TextField
                required
                maxLength={128}
                value={draft.id}
                disabled={draft.expectedRevision > 0}
                onChange={(event) =>
                  setDraft({ ...draft, id: event.target.value })
                }
              />
            </FormField>
            <FormField label={t.name}>
              <TextField
                required
                maxLength={128}
                value={draft.title}
                onChange={(event) =>
                  setDraft({ ...draft, title: event.target.value })
                }
              />
            </FormField>
          </FormRow>
          <FormField label={t.method}>
            <TextField
              required
              maxLength={128}
              value={draft.method.name}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  method: { ...draft.method, name: event.target.value },
                })
              }
            />
          </FormField>
          <FormField label={t.instructions}>
            <TextArea
              required
              maxLength={8192}
              value={draft.method.instructions}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  method: { ...draft.method, instructions: event.target.value },
                })
              }
            />
          </FormField>
          <fieldset>
            <legend>{t.claims}</legend>
            {claims
              .filter((claim) => claim.state === "current")
              .map((claim) => (
                <Checkbox
                  key={claim.id}
                  checked={draft.claims.includes(claim.id)}
                  onCheckedChange={(checked) =>
                    setDraft({
                      ...draft,
                      claims: checked
                        ? [...draft.claims, claim.id]
                        : draft.claims.filter((id) => id !== claim.id),
                    })
                  }
                  label={claim.statement || claim.id}
                />
              ))}
          </fieldset>
          {providers.length > 1 && (
            <FormField label={t.provider}>
              <Select
                label={t.provider}
                options={providers.map((id) => ({ value: id, label: id }))}
                value={draft.artifact.installationId || providers[0]}
                onValueChange={(installationId) =>
                  setDraft({
                    ...draft,
                    artifact: { ...draft.artifact, installationId },
                  })
                }
              />
            </FormField>
          )}
          <FormField label={t.artifact}>
            <TextField
              required
              pattern="[a-f0-9]{64}"
              value={draft.artifact.id}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  artifact: { ...draft.artifact, id: event.target.value },
                })
              }
            />
          </FormField>
          <FormField label={t.revision}>
            <TextField
              required
              pattern="[a-f0-9]{64}"
              value={draft.artifact.revision}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  artifact: { ...draft.artifact, revision: event.target.value },
                })
              }
            />
          </FormField>
          <FormField label={t.chart}>
            <TextField
              value={draft.artifact.object}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  artifact: { ...draft.artifact, object: event.target.value },
                })
              }
            />
          </FormField>
        </fieldset>
        <FormActions label={t.accept}>
          {recover ? (
            <Button
              type="button"
              pending={busy}
              onClick={() => void commit(recover.retry())}
            >
              {t.retry}
            </Button>
          ) : (
            <Button
              type="submit"
              pending={busy}
              disabled={providers.length === 0 || chosen.length === 0}
            >
              {t.accept}
            </Button>
          )}
        </FormActions>
      </form>
    </PanelSection>
  );
}
