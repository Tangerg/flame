import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import type { ReactNode } from "react";
import { Button, DropdownMenu, Icon, ProviderIcon, Surface } from "@/ui";
import {
  type ProviderConfiguration,
  setEmbeddingRole,
  setUtilityRole,
  useEmbeddingModelConfig,
  useProviderMutationMaterialGeneration,
  useUtilityModelConfig,
} from "../application/providerConfig";
import { useT } from "@/lib/i18n";
import { useAsyncFeedback } from "../../kit";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

const rs = stylex.create({
  title: { display: "flex", minWidth: 0, flexDirection: "column", gap: space.s1 },
  // A model id is long and the trigger is not: it truncates rather than widening the row.
  model: { maxWidth: "160px" },
  pickRow: { gridTemplateColumns: "16px minmax(0, 1fr) 14px", paddingInline: space.s2 },
});

function RoleSectionShell({
  title,
  description,
  error,
  note,
  children,
}: {
  title: string;
  description: string;
  error?: string | null;
  note?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Surface className={stylex.props(ss.stack).className}>
      <div {...stylex.props(ss.split)}>
        <div {...stylex.props(rs.title)}>
          <span {...stylex.props(ss.label, typeStep.uiMd)}>{title}</span>
          <span {...stylex.props(ss.hint, typeStep.uiMd)}>{description}</span>
        </div>
        {children}
      </div>
      {note}
      {error && <p {...stylex.props(ss.hint, ss.negative, typeStep.uiMd)}>{error}</p>}
    </Surface>
  );
}

export function UtilityModelSection() {
  const t = useT();
  const { role, modelOptions, selected, isSet, isAvailable, isError } = useUtilityModelConfig();
  const materialGeneration = useProviderMutationMaterialGeneration();
  const { feedback, run } = useAsyncFeedback(materialGeneration);
  const busy = feedback.state === "busy";

  const pick = (next: { provider: string; model: string } | null): Promise<void> =>
    run(() => setUtilityRole(next ?? {}), t("providers.utility.error"), wasGenerationRetired);

  return (
    <RoleSectionShell
      title={t("providers.utility.title")}
      description={t("providers.utility.desc")}
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
              disabled={busy}
              aria-label={t("providers.utility.title")}
            >
              {busy ? (
                <>
                  <Icon
                    name="loop"
                    size="xs"
                    className={stylex.props(ss.spin, ss.muted).className}
                  />
                  <span {...stylex.props(ss.muted)}>{t("providers.saving")}</span>
                </>
              ) : isSet && role?.provider ? (
                <>
                  <ProviderIcon provider={role.provider} size="sm" />
                  <span {...stylex.props(rs.model, ss.truncate, ss.mono, typeStep.uiSm)}>
                    {selected?.label ?? role.model}
                  </span>
                </>
              ) : (
                <span {...stylex.props(ss.muted)}>{t("providers.utility.main")}</span>
              )}
              {!busy && (
                <Icon name="chevron-down" size="xs" className={stylex.props(ss.muted).className} />
              )}
            </Button>
          }
        />
        <DropdownMenu.Content align="end" sideOffset={6}>
          <DropdownMenu.Item
            onClick={() => void pick(null)}
            className={stylex.props(rs.pickRow).className}
          >
            <span />
            <span {...stylex.props(ss.truncate)}>{t("providers.utility.main")}</span>
            {!isSet && (
              <Icon name="check" size="xs" className={stylex.props(ss.accent).className} />
            )}
          </DropdownMenu.Item>
          {modelOptions.map((m) => (
            <DropdownMenu.Item
              key={`${m.provider}:${m.id}`}
              onClick={() => void pick({ provider: m.provider, model: m.id })}
              className={stylex.props(rs.pickRow).className}
            >
              <ProviderIcon provider={m.provider} size="md" />
              <span {...stylex.props(ss.truncate)}>{m.label}</span>
              {role?.provider === m.provider && role?.model === m.id && (
                <Icon name="check" size="xs" className={stylex.props(ss.accent).className} />
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
  const { role, capableProviders, isSet, isAvailable } = useEmbeddingModelConfig();
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
              disabled={busy}
              aria-label={t("providers.embedding.title")}
            >
              {busy ? (
                <>
                  <Icon
                    name="loop"
                    size="xs"
                    className={stylex.props(ss.spin, ss.muted).className}
                  />
                  <span {...stylex.props(ss.muted)}>{t("providers.saving")}</span>
                </>
              ) : isSet && role?.provider ? (
                <>
                  <ProviderIcon provider={role.provider} size="sm" />
                  <span {...stylex.props(rs.model, ss.truncate, ss.mono, typeStep.uiSm)}>
                    {role.model}
                  </span>
                </>
              ) : (
                <span {...stylex.props(ss.muted)}>{t("providers.embedding.off")}</span>
              )}
              {!busy && (
                <Icon name="chevron-down" size="xs" className={stylex.props(ss.muted).className} />
              )}
            </Button>
          }
        />
        <DropdownMenu.Content align="end" sideOffset={6}>
          <DropdownMenu.Item
            onClick={() => void pick(null)}
            className={stylex.props(rs.pickRow).className}
          >
            <span />
            <span {...stylex.props(ss.truncate)}>{t("providers.embedding.off")}</span>
            {!isSet && (
              <Icon name="check" size="xs" className={stylex.props(ss.accent).className} />
            )}
          </DropdownMenu.Item>
          {capableProviders.map((p) => (
            <DropdownMenu.Item
              key={p.id}
              onClick={() => void pick(p)}
              className={stylex.props(rs.pickRow).className}
            >
              <ProviderIcon provider={p.id} size="md" />
              <span {...stylex.props(ss.truncate)}>
                {p.id}
                {p.defaultEmbeddingModel ? ` · ${p.defaultEmbeddingModel}` : ""}
              </span>
              {role?.provider === p.id && (
                <Icon name="check" size="xs" className={stylex.props(ss.accent).className} />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </RoleSectionShell>
  );
}
