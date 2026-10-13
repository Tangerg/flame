import { useState } from "react";
import { icons } from "@flame/icons/catalog";
import * as Drawings from "@flame/icons/react";
import {
  Metadata,
  Panel,
  PanelTitle,
  TextField,
  Tile,
  TileGrid,
  Toolbar,
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

export function Gallery() {
  const [filter, setFilter] = useState("");
  const query = filter.trim().toLocaleLowerCase();
  const visible = catalog.filter((icon) => icon.search.includes(query));
  return (
    <Panel label="Flame icon gallery">
      <Toolbar label="Icon filters">
        <PanelTitle>Flame icon gallery</PanelTitle>
        <TextField
          type="search"
          aria-label="Filter icons"
          placeholder="Name or category"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
        />
      </Toolbar>
      <Metadata>
        Original Flame drawings, shown with their canonical 24 px optical
        masters.
      </Metadata>
      <p role="status">
        {visible.length} of {catalog.length} icons. Immutable package content.
      </p>
      <TileGrid aria-label="Canonical icon drawings">
        {visible.map(({ Drawing, ...icon }) => (
          <Tile
            key={icon.name}
            title={icon.label}
            visual={<Drawing size={24} />}
          >
            <div>{icon.component}</div>
            <code>{icon.name}</code>
            <Metadata>{icon.group}</Metadata>
          </Tile>
        ))}
      </TileGrid>
    </Panel>
  );
}
