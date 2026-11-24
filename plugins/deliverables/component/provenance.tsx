import { type Translate } from "@flame/component-sdk";
import {
  Button,
  Disclosure,
  FormField,
  FormRow,
  Metadata,
  RecordBody,
  RecordGroup,
  TextArea,
  TextField,
} from "@flame/ui";
import { FIELD_LIMITS, LIMITS } from "../contracts/limits";
import { type Provenance } from "./types";

export function Source({ label, value }: { label: string; value: string }) {
  return (
    <Disclosure title={label}>
      <RecordBody>{value}</RecordBody>
    </Disclosure>
  );
}

export function ProvenanceEditor({
  value,
  change,
  disabled,
  t,
}: {
  value: Provenance;
  change(value: Provenance): void;
  disabled: boolean;
  t: Translate;
}) {
  return (
    <Disclosure title={t("provenance.title")} keepMounted>
      <Metadata>{t("provenance.help")}</Metadata>
      {value.sources.map((source, index) => (
        <RecordGroup key={index}>
          <FormRow>
            <FormField label={t("source.title")}>
              <TextField
                value={source.title}
                maxLength={FIELD_LIMITS.sourceTitle}
                disabled={disabled}
                onChange={(event) =>
                  change({
                    ...value,
                    sources: value.sources.map((item, at) =>
                      at === index
                        ? { ...item, title: event.target.value }
                        : item,
                    ),
                  })
                }
              />
            </FormField>
            <FormField label={t("source.uri")}>
              <TextField
                value={source.uri}
                maxLength={FIELD_LIMITS.uri}
                disabled={disabled}
                onChange={(event) =>
                  change({
                    ...value,
                    sources: value.sources.map((item, at) =>
                      at === index
                        ? { ...item, uri: event.target.value }
                        : item,
                    ),
                  })
                }
              />
            </FormField>
          </FormRow>
          <FormField label={t("source.note")}>
            <TextArea
              value={source.note}
              maxLength={FIELD_LIMITS.note}
              disabled={disabled}
              onChange={(event) =>
                change({
                  ...value,
                  sources: value.sources.map((item, at) =>
                    at === index ? { ...item, note: event.target.value } : item,
                  ),
                })
              }
            />
          </FormField>
          <Button
            disabled={disabled}
            onClick={() =>
              change({
                ...value,
                sources: value.sources.filter((_, at) => at !== index),
              })
            }
          >
            {t("source.remove")}
          </Button>
        </RecordGroup>
      ))}
      <Button
        disabled={disabled || value.sources.length >= LIMITS.provenanceSources}
        onClick={() =>
          change({
            ...value,
            sources: [...value.sources, { title: "", uri: "", note: "" }],
          })
        }
      >
        {t("source.add")}
      </Button>
      <FormField label={t("provenance.assumptions")}>
        <TextArea
          value={value.assumptions.join("\n")}
          disabled={disabled}
          onChange={(event) =>
            change({ ...value, assumptions: event.target.value.split("\n") })
          }
        />
      </FormField>
      <FormField label={t("provenance.units")}>
        <TextField
          value={value.units}
          maxLength={FIELD_LIMITS.units}
          disabled={disabled}
          onChange={(event) => change({ ...value, units: event.target.value })}
        />
      </FormField>
      <FormField label={t("provenance.reproduction")}>
        <TextArea
          value={value.reproduction}
          maxLength={FIELD_LIMITS.reproduction}
          disabled={disabled}
          onChange={(event) =>
            change({ ...value, reproduction: event.target.value })
          }
        />
      </FormField>
    </Disclosure>
  );
}

export function cleanProvenance(value: Provenance): Provenance {
  return {
    ...value,
    assumptions: value.assumptions.filter((item) => item.trim().length),
  };
}
