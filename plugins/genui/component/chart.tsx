import { useId } from "react";
import { Metadata, ScrollRegion } from "@flame/ui";

export interface ChartViewProps {
  kind: "line" | "bar" | "area" | "pie" | "donut" | "scatter";
  caption: string;
  labels: readonly string[];
  series: readonly { label: string; values: readonly number[] }[];
  xValues?: readonly number[];
  dataLabel: string;
  emptyLabel: string;
}

const DASHES = [undefined, "9 4", "2 4", "12 3 2 3", "6 3"] as const;
const WIDTH = 640;
const HEIGHT = 320;
const LEFT = 72;
const RIGHT = 616;
const TOP = 20;
const BOTTOM = 268;

function tick(value: number) {
  return Number(value.toPrecision(4)).toString();
}

function axisLabel(label: string) {
  return label.length > 14 ? `${label.slice(0, 13)}…` : label;
}

function Cartesian({ kind, labels, series, xValues }: ChartViewProps) {
  const all = series.flatMap((entry) => entry.values);
  const magnitude = Math.max(1, ...all.map(Math.abs));
  const lower = Math.min(0, ...all.map((value) => value / magnitude));
  const upper = Math.max(0, ...all.map((value) => value / magnitude));
  const span = upper - lower || 1;
  const step = (RIGHT - LEFT) / Math.max(1, labels.length);
  const xMagnitude = Math.max(1, ...(xValues ?? []).map(Math.abs));
  const normalizedX = xValues?.map((value) => value / xMagnitude) ?? [];
  const minimumX = normalizedX.length ? Math.min(...normalizedX) : 0;
  const maximumX = normalizedX.length ? Math.max(...normalizedX) : 1;
  const lowerX = minimumX === maximumX ? minimumX - 0.5 : minimumX;
  const spanX = maximumX - minimumX || 1;
  const x = (index: number) =>
    kind === "scatter"
      ? LEFT + ((normalizedX[index]! - lowerX) / spanX) * (RIGHT - LEFT)
      : LEFT + (index + 0.5) * step;
  const y = (value: number) =>
    BOTTOM - ((value / magnitude - lower) / span) * (BOTTOM - TOP);
  const baseline = y(0);
  const labelEvery = Math.max(1, Math.ceil(labels.length / 6));

  return (
    <>
      {Array.from({ length: 5 }, (_, index) => {
        const value = lower + (span * index) / 4;
        const position = BOTTOM - (index / 4) * (BOTTOM - TOP);
        return (
          <g key={index}>
            <line
              x1={LEFT}
              x2={RIGHT}
              y1={position}
              y2={position}
              stroke="currentColor"
              strokeOpacity={0.2}
            />
            <text
              x={LEFT - 8}
              y={position}
              textAnchor="end"
              dominantBaseline="middle"
              fill="currentColor"
              fontSize={12}
            >
              {tick(value * magnitude)}
            </text>
          </g>
        );
      })}
      {kind === "scatter"
        ? Array.from({ length: 5 }, (_, index) => (
            <text
              key={index}
              x={LEFT + (index / 4) * (RIGHT - LEFT)}
              y={BOTTOM + 24}
              textAnchor="middle"
              fill="currentColor"
              fontSize={12}
            >
              {tick((lowerX + (spanX * index) / 4) * xMagnitude)}
            </text>
          ))
        : labels.map((label, index) =>
            index % labelEvery === 0 ? (
              <text
                key={index}
                x={x(index)}
                y={BOTTOM + 24}
                textAnchor="middle"
                fill="currentColor"
                fontSize={12}
              >
                {axisLabel(label)}
              </text>
            ) : null,
          )}
      {series.map((entry, seriesIndex) => {
        const points = entry.values
          .map((value, index) => `${x(index)},${y(value)}`)
          .join(" ");
        return (
          <g key={entry.label}>
            <title>{entry.label}</title>
            {kind === "area" && entry.values.length > 0 && (
              <polygon
                points={`${x(0)},${baseline} ${points} ${x(entry.values.length - 1)},${baseline}`}
                fill="currentColor"
                fillOpacity={0.08 + (seriesIndex % 3) * 0.05}
              />
            )}
            {kind !== "bar" && kind !== "scatter" && (
              <polyline
                points={points}
                fill="none"
                stroke="currentColor"
                strokeWidth={2}
                strokeDasharray={DASHES[seriesIndex % DASHES.length]}
              />
            )}
            {entry.values.map((value, index) => {
              const barWidth = (step * 0.75) / series.length;
              const tooltip = `${entry.label} · ${labels[index]}: ${value}`;
              return kind === "bar" ? (
                <rect
                  key={index}
                  x={x(index) - step * 0.375 + seriesIndex * barWidth}
                  y={Math.min(baseline, y(value))}
                  width={Math.max(1, barWidth - 1)}
                  height={Math.abs(y(value) - baseline)}
                  fill="currentColor"
                  fillOpacity={0.3 + (seriesIndex % 5) * 0.14}
                  stroke="currentColor"
                  strokeWidth={0.5}
                >
                  <title>{tooltip}</title>
                </rect>
              ) : (
                <circle
                  key={index}
                  cx={x(index)}
                  cy={y(value)}
                  r={3 + (seriesIndex % 2)}
                  fill="currentColor"
                >
                  <title>{tooltip}</title>
                </circle>
              );
            })}
          </g>
        );
      })}
    </>
  );
}

function Pie({
  kind,
  labels,
  series,
  patternId,
}: ChartViewProps & { patternId: string }) {
  const values = series[0]?.values ?? [];
  const magnitude = Math.max(1, ...values);
  const total = values.reduce((sum, value) => sum + value / magnitude, 0);
  let angle = -Math.PI / 2;
  const centerX = 215;
  const centerY = 155;
  const radius = 126;
  const innerRadius = kind === "donut" ? radius * 0.55 : 0;

  return (
    <>
      <defs>
        {labels.map((_, index) => (
          <pattern
            key={index}
            id={`${patternId}-${index}`}
            patternUnits="userSpaceOnUse"
            width={6 + (index % 4) * 3}
            height={6 + (index % 4) * 3}
            patternTransform={`rotate(${index * 37})`}
          >
            <rect
              width="100%"
              height="100%"
              fill="currentColor"
              fillOpacity={0.05}
            />
            <path d="M 0 0 L 0 20" stroke="currentColor" strokeWidth={2} />
          </pattern>
        ))}
      </defs>
      {values.map((value, index) => {
        const fraction = total === 0 ? 0 : value / magnitude / total;
        const start = angle;
        angle += fraction * 2 * Math.PI;
        const fill = `url(#${patternId}-${index})`;
        const tooltip = `${labels[index]}: ${value} (${tick(fraction * 100)}%)`;
        return (
          <g key={index}>
            {fraction >= 1 ? (
              <circle
                cx={centerX}
                cy={centerY}
                r={innerRadius ? (radius + innerRadius) / 2 : radius}
                fill={innerRadius ? "none" : fill}
                stroke={innerRadius ? fill : "currentColor"}
                strokeWidth={innerRadius ? radius - innerRadius : 1}
              >
                <title>{tooltip}</title>
              </circle>
            ) : fraction > 0 ? (
              <path
                d={`M ${centerX + radius * Math.cos(start)} ${centerY + radius * Math.sin(start)} A ${radius} ${radius} 0 ${fraction > 0.5 ? 1 : 0} 1 ${centerX + radius * Math.cos(angle)} ${centerY + radius * Math.sin(angle)} L ${centerX + innerRadius * Math.cos(angle)} ${centerY + innerRadius * Math.sin(angle)} ${innerRadius ? `A ${innerRadius} ${innerRadius} 0 ${fraction > 0.5 ? 1 : 0} 0 ${centerX + innerRadius * Math.cos(start)} ${centerY + innerRadius * Math.sin(start)}` : ""} Z`}
                fill={fill}
                stroke="currentColor"
                strokeWidth={1}
              >
                <title>{tooltip}</title>
              </path>
            ) : null}
            {index < 10 && (
              <g transform={`translate(380 ${30 + index * 27})`}>
                <rect
                  width={14}
                  height={14}
                  fill={fill}
                  stroke="currentColor"
                />
                <text x={24} y={11} fill="currentColor" fontSize={12}>
                  {axisLabel(labels[index]!)} · {tick(fraction * 100)}%
                </text>
              </g>
            )}
          </g>
        );
      })}
    </>
  );
}

export function ChartView(props: ChartViewProps) {
  const id = useId().replaceAll(":", "");
  const { kind, caption, labels, series, dataLabel, emptyLabel } = props;
  return (
    <figure style={{ margin: 0, minWidth: 0 }}>
      <figcaption>{caption}</figcaption>
      {labels.length === 0 || series.length === 0 ? (
        <Metadata>{emptyLabel}</Metadata>
      ) : (
        <svg
          role="img"
          aria-label={caption}
          viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
          style={{
            display: "block",
            width: "100%",
            height: "auto",
            maxHeight: "28rem",
          }}
        >
          {kind === "pie" || kind === "donut" ? (
            <Pie {...props} patternId={id} />
          ) : (
            <Cartesian {...props} />
          )}
        </svg>
      )}
      <ScrollRegion label={dataLabel}>
        <table>
          <caption style={{ textAlign: "start" }}>{dataLabel}</caption>
          <thead>
            <tr>
              <th scope="col">{caption}</th>
              {kind === "scatter" && <th scope="col">x</th>}
              {series.map((entry, index) => (
                <th key={entry.label} scope="col">
                  {kind !== "pie" && kind !== "donut" && (
                    <svg width={24} height={8} aria-hidden="true">
                      <line
                        x1={0}
                        x2={24}
                        y1={4}
                        y2={4}
                        stroke="currentColor"
                        strokeWidth={2}
                        strokeDasharray={DASHES[index % DASHES.length]}
                      />
                    </svg>
                  )}{" "}
                  {entry.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {labels.map((label, index) => (
              <tr key={index}>
                <th scope="row">{label}</th>
                {kind === "scatter" && (
                  <td>{String(props.xValues?.[index])}</td>
                )}
                {series.map((entry) => (
                  <td key={entry.label}>{String(entry.values[index])}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </ScrollRegion>
    </figure>
  );
}
