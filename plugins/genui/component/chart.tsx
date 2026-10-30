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
const LABEL_SIZE = 12;
const LEFT = 72;
const RIGHT = 616;
const TOP = 20;
const BOTTOM = 268;

const MARK_PATHS = [
  undefined,
  "M -3.5 -3.5 H 3.5 V 3.5 H -3.5 Z",
  "M 0 -4.5 L 4.5 0 L 0 4.5 L -4.5 0 Z",
  "M 0 -4.5 L 4 3.5 L -4 3.5 Z",
  "M -4 -3.5 L 4 -3.5 L 0 4.5 Z",
  "M 0 -5 L 1.5 -1.6 L 4.8 -1.5 L 2.3 1 L 3 4.5 L 0 2.5 L -3 4.5 L -2.3 1 L -4.8 -1.5 L -1.5 -1.6 Z",
] as const;

function PointMark({
  index,
  x,
  y,
  title,
}: {
  index: number;
  x: number;
  y: number;
  title?: string;
}) {
  const path = MARK_PATHS[index % MARK_PATHS.length];
  const paint = {
    fill: index < MARK_PATHS.length ? "currentColor" : "none",
    stroke: "currentColor",
    strokeWidth: 1.25,
    strokeLinejoin: "round",
  } as const;
  return path ? (
    <g transform={`translate(${x} ${y})`} {...paint}>
      {title && <title>{title}</title>}
      <path d={path} />
    </g>
  ) : (
    <circle cx={x} cy={y} r={4} {...paint}>
      {title && <title>{title}</title>}
    </circle>
  );
}

function ChartPatterns({ id, count }: { id: string; count: number }) {
  return (
    <defs>
      {Array.from({ length: count }, (_, index) => (
        <pattern
          key={index}
          id={`${id}-${index}`}
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
  );
}

function tick(value: number) {
  return Number(value.toPrecision(4)).toString();
}

function axisLabel(label: string) {
  return label.length > 14 ? `${label.slice(0, 13)}…` : label;
}

function Cartesian({
  kind,
  labels,
  series,
  xValues,
  patternId,
}: ChartViewProps & { patternId: string }) {
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
      {kind === "bar" && <ChartPatterns id={patternId} count={series.length} />}
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
              fontSize={LABEL_SIZE}
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
              fontSize={LABEL_SIZE}
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
                fontSize={LABEL_SIZE}
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
                  fill={`url(#${patternId}-${seriesIndex})`}
                  stroke="currentColor"
                  strokeWidth={0.5}
                >
                  <title>{tooltip}</title>
                </rect>
              ) : (
                <PointMark
                  key={index}
                  index={seriesIndex}
                  x={x(index)}
                  y={y(value)}
                  title={tooltip}
                />
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
      <ChartPatterns id={patternId} count={labels.length} />
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
                <text x={24} y={11} fill="currentColor" fontSize={LABEL_SIZE}>
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
        <ScrollRegion label={caption}>
          <svg
            role="img"
            aria-label={caption}
            viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
            style={{
              display: "block",
              width: "100%",
              height: "auto",
              minWidth: `calc(var(--fs-ui-sm) * ${WIDTH} / ${LABEL_SIZE})`,
              maxWidth: "56rem",
            }}
          >
            {kind === "pie" || kind === "donut" ? (
              <Pie {...props} patternId={id} />
            ) : (
              <Cartesian {...props} patternId={id} />
            )}
          </svg>
        </ScrollRegion>
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
                    <svg width={24} height={16} aria-hidden="true">
                      {kind === "bar" ? (
                        <rect
                          x={3}
                          y={2}
                          width={18}
                          height={12}
                          fill={`url(#${id}-${index})`}
                          stroke="currentColor"
                          strokeWidth={0.5}
                        />
                      ) : (
                        <>
                          {kind !== "scatter" && (
                            <line
                              x1={0}
                              x2={24}
                              y1={8}
                              y2={8}
                              stroke="currentColor"
                              strokeWidth={2}
                              strokeDasharray={DASHES[index % DASHES.length]}
                            />
                          )}
                          <PointMark index={index} x={12} y={8} />
                        </>
                      )}
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
