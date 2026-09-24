import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import type { ReactNode } from "react";
import { Button, DropdownMenu, Icon, ProviderIcon, vocab } from "@/ui";
import {
  type ProviderConfiguration,
  type RoleConfigState,
  setEmbeddingRole,
  setUtilityRole,
  useEmbeddingModelConfig,
  useProviderMutationMaterialGeneration,
  useUtilityModelConfig,
} from "../application/providerConfig";
import { useT } from "@/lib/i18n";
import { useAsyncFeedback } from "../../kit";
import { face, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

const rs = stylex.create({
  title: { display: "flex", minWidth: 0, flexDirection: "column", gap: space.s1 },
  model: { maxWidth: "160px" },
  row: {
    display: "flex",
    flexDirection: "column",
    gap: space.s3,
    borderTopWidth: { default: "var(--control-edge-width)", ":first-child": 0 },
    borderTopStyle: "solid",
    borderTopColor: surface.field,
    paddingInline: space.s4,
    paddingBlock: space.s3,
  },
});

function RoleSectionShell({
  title,
  description,
  state,
  error,
  note,
  children,
}: {
  title: string;
  description: string;
  state: RoleConfigState;
  error?: string | null;
  note?: ReactNode;
  children: ReactNode;
}) {
  const t = useT();
  return (
    <div {...stylex.props(rs.row)}>
      <div {...stylex.props(ss.split)}>
        <div {...stylex.props(rs.title)}>
          <span {...stylex.props(ss.label, typeStep.uiMd)}>{title}</span>
          <span {...stylex.props(ss.hint, typeStep.uiMd)}>{description}</span>
        </div>
        {state.kind === "loading" ? (
          <Button
            type="button"
            variant="outline"
            size="md"
            press="none"
            disabled
            aria-label={title}
          >
            <span {...stylex.props(vocab.muted)}>{t("common.loading")}</span>
          </Button>
        ) : state.kind === "error" ? (
          <Button type="button" variant="outline" size="md" onClick={state.retry}>
            {t("common.retry")}
          </Button>
        ) : (
          children
        )}
      </div>
      {state.kind === "ready" && note}
      {state.kind === "error" && (
        <p role="alert" {...stylex.props(ss.hint, vocab.negative, typeStep.uiMd)}>
          {t("providers.role.loadError")}
        </p>
      )}
      {state.kind === "ready" && state.stale && (
        <p {...stylex.props(ss.hint, typeStep.uiMd)}>{t("providers.role.stale")}</p>
      )}
      {error && (
        <p role="alert" {...stylex.props(ss.hint, vocab.negative, typeStep.uiMd)}>
          {error}
        </p>
      )}
    </div>
  );
}

export function UtilityModelSection() {
  const t = useT();
  const { role, modelOptions, selected, isSet, isAvailable, isError, state } =
    useUtilityModelConfig();
  const materialGeneration = useProviderMutationMaterialGeneration();
  const { feedback, run } = useAsyncFeedback(materialGeneration);
  const busy = feedback.state === "busy";

  const pick = (next: { provider: string; model: string } | null): Promise<void> =>
    run(() => setUtilityRole(next ?? {}), t("providers.utility.error"), wasGenerationRetired);

  return (
    <RoleSectionShell
      title={t("providers.utility.title")}
      description={t("providers.utility.desc")}
      state={state}
      error={
        feedback.state === "error" ? feedback.reason : isError ? t("providers.models.error") : null
      }
      note={
        isSet && !isAvailable ? (
          <p {...stylex.props(ss.hint, typeStep.uiMd)}>{t("providers.notConfigured")}</p>
        ) : null
      }
    >
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <Button
              type="button"
              variant="outline"
              size="md"
              press="none"
              pending={busy}
              aria-label={t("providers.utility.title")}
            >
              {busy ? (
                <>
                  <Icon
                    name="loop"
                    size="xs"
                    className={stylex.props(ss.spin, vocab.muted).className}
                  />
                  <span {...stylex.props(vocab.muted)}>{t("providers.saving")}</span>
                </>
              ) : isSet && role?.provider ? (
                <>
                  <ProviderIcon provider={role.provider} size="sm" />
                  <span {...stylex.props(rs.model, vocab.truncate, typeStep.uiSm, face.mono)}>
                    {selected?.label ?? role.model}
                  </span>
                </>
              ) : (
                <span {...stylex.props(vocab.muted)}>{t("providers.utility.main")}</span>
              )}
              {!busy && (
                <Icon
                  name="chevron-down"
                  size="xs"
                  className={stylex.props(vocab.muted).className}
                />
              )}
            </Button>
          }
        />
        <DropdownMenu.Content align="end" sideOffset={6}>
          <DropdownMenu.Item onClick={() => void pick(null)} layout="pick">
            <span />
            <span {...stylex.props(vocab.truncate)}>{t("providers.utility.main")}</span>
            {!isSet && (
              <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
            )}
          </DropdownMenu.Item>
          {modelOptions.map((m) => (
            <DropdownMenu.Item
              key={`${m.provider}:${m.id}`}
              onClick={() => void pick({ provider: m.provider, model: m.id })}
              layout="pick"
            >
              <ProviderIcon provider={m.provider} size="md" />
              <span {...stylex.props(vocab.truncate)}>{m.label}</span>
              {role?.provider === m.provider && role?.model === m.id && (
                <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </RoleSectionShell>
  );
}

export function EmbeddingModelSection() {
  const t = useT();
  const { role, capableProviders, isSet, isAvailable, state } = useEmbeddingModelConfig();
  const materialGeneration = useProviderMutationMaterialGeneration();
  const { feedback, run } = useAsyncFeedback(materialGeneration);
  const busy = feedback.state === "busy";

  const pick = (p: ProviderConfiguration | null): Promise<void> =>
    run(
      () => setEmbeddingRole(p ? { provider: p.id, model: p.defaultEmbeddingModel || "" } : {}),
      t("providers.embedding.error"),
    );

  return (
    <RoleSectionShell
      title={t("providers.embedding.title")}
      description={t("providers.embedding.desc")}
      state={state}
      error={feedback.state === "error" ? feedback.reason : null}
      note={
        isSet && !isAvailable ? (
          <p {...stylex.props(ss.hint, typeStep.uiMd)}>{t("providers.notConfigured")}</p>
        ) : capableProviders.length === 0 ? (
          <p {...stylex.props(ss.hint, typeStep.uiMd)}>{t("providers.embedding.none")}</p>
        ) : null
      }
    >
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <Button
              type="button"
              variant="outline"
              size="md"
              press="none"
              pending={busy}
              aria-label={t("providers.embedding.title")}
            >
              {busy ? (
                <>
                  <Icon
                    name="loop"
                    size="xs"
                    className={stylex.props(ss.spin, vocab.muted).className}
                  />
                  <span {...stylex.props(vocab.muted)}>{t("providers.saving")}</span>
                </>
              ) : isSet && role?.provider ? (
                <>
                  <ProviderIcon provider={role.provider} size="sm" />
                  <span {...stylex.props(rs.model, vocab.truncate, typeStep.uiSm, face.mono)}>
                    {role.model}
                  </span>
                </>
              ) : (
                <span {...stylex.props(vocab.muted)}>{t("providers.embedding.off")}</span>
              )}
              {!busy && (
                <Icon
                  name="chevron-down"
                  size="xs"
                  className={stylex.props(vocab.muted).className}
                />
              )}
            </Button>
          }
        />
        <DropdownMenu.Content align="end" sideOffset={6}>
          <DropdownMenu.Item onClick={() => void pick(null)} layout="pick">
            <span />
            <span {...stylex.props(vocab.truncate)}>{t("providers.embedding.off")}</span>
            {!isSet && (
              <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
            )}
          </DropdownMenu.Item>
          {capableProviders.map((p) => (
            <DropdownMenu.Item key={p.id} onClick={() => void pick(p)} layout="pick">
              <ProviderIcon provider={p.id} size="md" />
              <span {...stylex.props(vocab.truncate)}>
                {p.id}
                {p.defaultEmbeddingModel ? ` · ${p.defaultEmbeddingModel}` : ""}
              </span>
              {role?.provider === p.id && (
                <Icon name="check" size="xs" className={stylex.props(vocab.accent).className} />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </RoleSectionShell>
  );
}
