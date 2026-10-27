import { useEffect, useState } from "react";
import type { ComponentContext, PluginComponent } from "@flame/component-sdk";
import type {
  PluginInstallation,
  PluginPointReference,
} from "@flame/runtime-client/wire";
import { Button } from "@flame/ui/button";
import { Panel, PanelTitle } from "@flame/ui";

export function CompositionBoard({ context }: { context: ComponentContext }) {
  const [catalog, setCatalog] = useState<{
    rows?: PluginInstallation[];
    error?: string;
  }>({});
  const [selected, setSelected] = useState<PluginPointReference>();
  useEffect(() => {
    const controller = new AbortController();
    const signal = AbortSignal.any([controller.signal, context.signal]);
    const observe = async () => {
      const subscription = await context.runtime.runtimeEvents.subscribe(
        { topics: ["plugins.changed"] },
        signal,
      );
      const events = subscription.events[Symbol.asyncIterator]();
      try {
        const read = async () => {
          const { data } = await context.runtime.plugins.list(signal);
          if (!signal.aborted) setCatalog({ rows: data });
        };
        await read();
        while (!(await events.next()).done) await read();
      } finally {
        await events.return?.();
      }
    };
    void context
      .track(() => observe())
      .catch((error: unknown) => {
        if (!signal.aborted)
          setCatalog({
            error: error instanceof Error ? error.message : String(error),
          });
      });
    return () => controller.abort();
  }, [context]);
  if (catalog.error) return <p role="status">{catalog.error}</p>;
  if (!catalog.rows) return <p role="status">Loading composition</p>;
  const owner = catalog.rows.find(
    (row) => row.id === context.component.installationId,
  );
  if (!owner) return <p role="status">The consumer is unavailable.</p>;
  const contract = (name: string) => {
    const point = owner.selected.points.find((entry) => entry.id === name);
    if (!point) throw new Error(`Missing declared point: ${name}`);
    return point.contract;
  };
  const candidates = (name: string) =>
    catalog
      .rows!.filter((row) => row.presentation === "admitted")
      .flatMap((row) =>
        row.selected.pointValues
          .filter((entry) => entry.contract === contract(name))
          .map((entry) => ({ installationId: row.id, contribution: entry.id })),
      )
      .sort((a, b) =>
        `${a.installationId}/${a.contribution}`.localeCompare(
          `${b.installationId}/${b.contribution}`,
        ),
      );
  const target = { scope: "installation", reference: owner.id };
  const summary =
    selected &&
    candidates("summary").some(
      (entry) =>
        entry.installationId === selected.installationId &&
        entry.contribution === selected.contribution,
    )
      ? [selected]
      : [];
  return (
    <Panel label="Plugin composition">
      <PanelTitle>Plugin composition</PanelTitle>
      {candidates("summary").map((entry) => (
        <Button
          key={`${entry.installationId}/${entry.contribution}`}
          onClick={() => setSelected(entry)}
        >
          Use contributed summary
        </Button>
      ))}
      {context.compose({
        contract: contract("summary"),
        target,
        attributes: {},
        order: summary,
      })}
      {context.compose({
        contract: contract("frames"),
        target,
        attributes: {},
        order: candidates("frames"),
        children: context.compose({
          contract: contract("panels"),
          target,
          attributes: {},
          input: { title: "Inspected board" },
          children: <p>Consumer content</p>,
        }),
      })}
    </Panel>
  );
}

export const Inspector: PluginComponent<
  { title: string },
  { label: string }
> = ({ input, contribution }) => (
  <p>
    {contribution?.label}: {input?.title}
  </p>
);
