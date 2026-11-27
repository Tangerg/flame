import type { Translate } from "@flame/component-sdk";
import { Metadata, RecordBody, RecordGroup } from "@flame/ui";
import type { MethodOutputs } from "../package/io.github.tangerg.flame/contracts/methods.generated";

type Version = NonNullable<
  MethodOutputs["flame.github/review-work.v1"]["selected"]
>;
type Project = NonNullable<NonNullable<Version["recheck"]>["project"]>;

export function ProjectCheckEvidence({
  project,
  t,
}: {
  project: Project;
  t: Translate;
}) {
  return (
    <RecordGroup aria-label={t("projectChecks")}>
      <Metadata>
        {t("projectChecks")} · {project.environment} · {project.runner}
      </Metadata>
      {project.checks.map((check) => (
        <RecordGroup key={check.name}>
          <Metadata>
            {check.name} · {t("projectCheck." + check.status)}
            {check.exitCode === undefined ? null : (
              <> · {t("exitCode", { code: String(check.exitCode) })}</>
            )}
          </Metadata>
          <RecordBody>
            <code className="break-words whitespace-pre-wrap">
              {check.command}
            </code>
          </RecordBody>
          {check.log && (
            <details>
              <summary>{t("checkOutput")}</summary>
              <EvidenceText
                value={check.log}
                truncated={check.logTruncated}
                t={t}
              />
            </details>
          )}
        </RecordGroup>
      ))}
    </RecordGroup>
  );
}

export function ReviewPatch({
  patch,
  truncated,
  t,
}: {
  patch: string;
  truncated?: boolean;
  t: Translate;
}) {
  return (
    <details>
      <summary>{t("findingPatch")}</summary>
      <EvidenceText value={patch} truncated={truncated} t={t} />
    </details>
  );
}

function EvidenceText({
  value,
  truncated,
  t,
}: {
  value: string;
  truncated?: boolean;
  t: Translate;
}) {
  return (
    <RecordBody>
      <pre className="break-words whitespace-pre-wrap">{value}</pre>
      {truncated && <Metadata>{t("evidenceTruncated")}</Metadata>}
    </RecordBody>
  );
}
