import { useState } from "react";
import { icons } from "@flame/icons/catalog";
import * as Drawings from "@flame/icons/react";
import {
  Metadata,
  EmptyState,
  PanelSection,
  Select,
  Panel,
  PanelHeader,
  SearchField,
  Tile,
  TileGrid,
  FilterBar,
} from "@flame/ui";

const catalog = icons.map((icon) => {
  const Drawing = Drawings[icon.component as keyof typeof Drawings];
  if (!Drawing)
    throw new Error(`icon export is unavailable: ${icon.component}`);
  return {
    ...icon,
    Drawing,
    search: [icon.name, icon.component, icon.label, icon.group, ...icon.tags]
      .join(" ")
      .toLocaleLowerCase(),
  };
});

const groups = [...new Set(catalog.map((icon) => icon.group))];

export function Gallery() {
  const [filter, setFilter] = useState("");
  const [group, setGroup] = useState("");
  const query = filter.trim().toLocaleLowerCase();
  const visible = catalog.filter(
    (icon) => (!group || icon.group === group) && icon.search.includes(query),
  );
  return (
    <Panel
      label="Flame icon gallery"
      header={
        <PanelHeader
          title="Flame icon gallery"
          description="Original drawings, shown at their canonical 24 px optical size."
        />
      }
      controls={
        <FilterBar
          label="Icon filters"
          search={
            <SearchField
              size="lg"
              aria-label="Filter icons"
              placeholder="Name or category"
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
            />
          }
        >
          <Select
            size="lg"
            label="Icon category"
            value={group}
            onValueChange={setGroup}
            options={[
              { value: "", label: "All categories" },
              ...groups.map((value) => ({ value, label: value })),
            ]}
          />
        </FilterBar>
      }
    >
      <Metadata role="status">
        {visible.length} of {catalog.length} icons
      </Metadata>
      {groups.map((category) => {
        const items = visible.filter((icon) => icon.group === category);
        if (!items.length) return null;
        return (
          <PanelSection
            key={category}
            title={category}
            description={`${items.length} icons`}
          >
            <TileGrid aria-label={`${category} icon drawings`}>
              {items.map(({ Drawing, ...icon }) => (
                <Tile
                  key={icon.name}
                  title={icon.label}
                  visual={<Drawing size={24} />}
                >
                  <strong>{icon.label}</strong>
                  <Metadata>
                    <code>{icon.component}</code>
                    <br />
                    <code>{icon.name}</code>
                  </Metadata>
                </Tile>
              ))}
            </TileGrid>
          </PanelSection>
        );
      })}
      {!visible.length && (
        <EmptyState
          title="No matching icons"
          sub="Try another name or category."
        />
      )}
    </Panel>
  );
}
