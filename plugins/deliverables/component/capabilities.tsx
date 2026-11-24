import { failureText, type Translate } from "@flame/component-sdk";
import { Callout, Disclosure, Facts, Metadata } from "@flame/ui";
import {
  formatCatalogSchema,
  inspectionSchema,
  type Inspection,
} from "../contracts/content";
import { engineCatalogSchema } from "../contracts/evidence";
import { Source } from "./provenance";
import { type DeliverablesState } from "./useDeliverables";
import type { DocumentRead } from "./types";

export function readCatalog(
  value: NonNullable<DeliverablesState["catalog"]>["value"],
) {
  if (!value) return;
  try {
    return {
      formats: formatCatalogSchema.parse(JSON.parse(value.formatsJSON)),
      engines: engineCatalogSchema.parse(JSON.parse(value.enginesJSON)),
    };
  } catch (error) {
    return { error: failureText(error) };
  }
}

export function inspectRevision(document: DocumentRead | undefined): {
  value?: Inspection;
  error?: string;
} {
  if (!document) return {};
  try {
    const value = inspectionSchema.parse(JSON.parse(document.inspectionJSON));
    if (value.format !== document.revision.format)
      throw new Error(
        "deliverables: inspection format differs from its native revision",
      );
    return { value };
  } catch (error) {
    return { error: failureText(error) };
  }
}

export function Capabilities({
  catalog,
  limits,
  t,
}: {
  catalog: ReturnType<typeof readCatalog>;
  limits?: string;
  t: Translate;
}) {
  return (
    <Disclosure title={t("capabilities.title")}>
      {catalog?.engines && (
        <>
          <Facts
            values={[
              [t("preview.platform"), catalog.engines.platform],
              [
                t("capabilities.configured"),
                t(catalog.engines.configured ? "yes" : "no"),
              ],
            ]}
          />
          {!catalog.engines.configured && (
            <Callout kind="note">{t("capabilities.setup")}</Callout>
          )}
          <Source
            label={t("capabilities.engines")}
            value={JSON.stringify(catalog.engines.configuration, null, 2)}
          />
          {catalog.engines.limitations.map((limitation) => (
            <Metadata key={limitation}>{limitation}</Metadata>
          ))}
        </>
      )}
      {catalog?.formats?.formats.map((entry) => (
        <Disclosure key={entry.format} title={t(`format.${entry.format}`)}>
          {entry.limitations.map((limitation) => (
            <Metadata key={limitation}>{limitation}</Metadata>
          ))}
          {entry.createSchemaJSON && (
            <Source label={t("author.schema")} value={entry.createSchemaJSON} />
          )}
          <Source label={t("edit.schema")} value={entry.editSchemaJSON} />
        </Disclosure>
      ))}
      {limits !== undefined && (
        <Source label={t("capabilities.limits")} value={limits} />
      )}
    </Disclosure>
  );
}
