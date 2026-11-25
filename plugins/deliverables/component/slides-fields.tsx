import { useEffect, useState } from "react";
import { type Translate } from "@flame/component-sdk";
import {
  Button,
  Callout,
  FormField,
  RecordGroup,
  TextArea,
  TextField,
} from "@flame/ui";
import { type ChartData, type Frame } from "../contracts/content";
import { type ElementView } from "../contracts/layouts";
import { EMU_PER_INCH, inches } from "./slides-presentation";

export function FrameFields({
  frame,
  editable,
  onCommit,
  t,
}: {
  frame: Frame;
  editable: boolean;
  onCommit(frame: Frame): void;
  t: Translate;
}) {
  const fields = ["x", "y", "width", "height"] as const;
  const [values, setValues] = useState(() =>
    Object.fromEntries(fields.map((field) => [field, inches(frame[field])])),
  );
  const [invalid, setInvalid] = useState(false);
  useEffect(() => {
    setValues(
      Object.fromEntries(fields.map((field) => [field, inches(frame[field])])),
    );
    setInvalid(false);
  }, [frame.x, frame.y, frame.width, frame.height]);
  const commit = () => {
    const next = Object.fromEntries(
      fields.map((field) => [
        field,
        Math.round(Number(values[field]) * EMU_PER_INCH),
      ]),
    ) as Frame;
    if (
      fields.some(
        (field) =>
          values[field]!.trim() === "" || !Number.isSafeInteger(next[field]),
      ) ||
      next.width < 0 ||
      next.height < 0
    ) {
      setInvalid(true);
      return;
    }
    setInvalid(false);
    if (fields.some((field) => next[field] !== frame[field])) onCommit(next);
  };
  return (
    <RecordGroup>
      {fields.map((field) => (
        <FormField key={field} label={t(`slides.frame.${field}`)}>
          <TextField
            type="number"
            step="0.01"
            inputMode="decimal"
            value={values[field]}
            readOnly={!editable}
            invalid={invalid}
            onChange={(event) =>
              setValues({ ...values, [field]: event.target.value })
            }
            onBlur={commit}
            onKeyDown={(event) => {
              if (event.key === "Enter") commit();
            }}
          />
        </FormField>
      ))}
      {invalid && <Callout kind="warning">{t("slides.frame.invalid")}</Callout>}
    </RecordGroup>
  );
}

export function RunField({
  label: title,
  text,
  editable,
  onCommit,
}: {
  label: string;
  text: string;
  editable: boolean;
  onCommit(text: string): void;
}) {
  const [value, setValue] = useState(text);
  useEffect(() => setValue(text), [text]);
  return (
    <FormField label={title}>
      <TextArea
        autosize
        size="sm"
        value={value}
        readOnly={!editable}
        onChange={(event) => setValue(event.target.value)}
        onBlur={() => {
          if (value !== text) onCommit(value);
        }}
      />
    </FormField>
  );
}

export function ChartFields({
  chart,
  editable,
  onCommit,
  t,
}: {
  chart: NonNullable<ElementView["chart"]>;
  editable: boolean;
  onCommit(data: ChartData): void;
  t: Translate;
}) {
  const source = (data: ChartData) => ({
    categories: [...data.categories],
    names: data.series.map((series) => series.name),
    values: data.series.map((series) =>
      series.values.map((value) => (value === null ? "" : String(value))),
    ),
  });
  const [grid, setGrid] = useState(() => source(chart.data));
  const [invalid, setInvalid] = useState(false);
  useEffect(() => {
    setGrid(source(chart.data));
    setInvalid(false);
  }, [chart.data]);
  const writable = editable && chart.workbook !== "mixed";
  const apply = () => {
    const values = grid.values.map((series) =>
      series.map((value) => (value.trim() === "" ? null : Number(value))),
    );
    if (
      values.some((series) =>
        series.some((value) => value !== null && !Number.isFinite(value)),
      )
    ) {
      setInvalid(true);
      return;
    }
    setInvalid(false);
    onCommit({
      categories: grid.categories,
      series: grid.names.map((name, index) => ({
        name,
        values: values[index]!,
      })),
    });
  };
  const cell = { padding: "2px 4px", minWidth: "5rem" };
  return (
    <RecordGroup>
      {chart.workbook === "mixed" && (
        <Callout kind="note">{t("slides.chart.mixed")}</Callout>
      )}
      <div style={{ overflowX: "auto" }}>
        <table style={{ borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th scope="col" style={cell}>
                {t("slides.chart.category")}
              </th>
              {grid.names.map((name, series) => (
                <th key={series} scope="col" style={cell}>
                  <TextField
                    size="sm"
                    aria-label={t("slides.chart.series", {
                      number: series + 1,
                    })}
                    value={name}
                    readOnly={!writable}
                    onChange={(event) => {
                      const names = [...grid.names];
                      names[series] = event.target.value;
                      setGrid({ ...grid, names });
                    }}
                  />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {grid.categories.map((category, point) => (
              <tr key={point}>
                <th scope="row" style={cell}>
                  <TextField
                    size="sm"
                    aria-label={t("slides.chart.categoryNumber", {
                      number: point + 1,
                    })}
                    value={category}
                    readOnly={!writable}
                    onChange={(event) => {
                      const categories = [...grid.categories];
                      categories[point] = event.target.value;
                      setGrid({ ...grid, categories });
                    }}
                  />
                </th>
                {grid.values.map((series, index) => (
                  <td key={index} style={cell}>
                    <TextField
                      size="sm"
                      inputMode="decimal"
                      aria-label={`${grid.names[index]} · ${category}`}
                      value={series[point]}
                      readOnly={!writable}
                      invalid={invalid}
                      onChange={(event) => {
                        const values = grid.values.map((entry) => [...entry]);
                        values[index]![point] = event.target.value;
                        setGrid({ ...grid, values });
                      }}
                    />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {invalid && <Callout kind="warning">{t("slides.chart.invalid")}</Callout>}
      <Button disabled={!writable} onClick={apply}>
        {t("slides.chart.apply")}
      </Button>
    </RecordGroup>
  );
}
