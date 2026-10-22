import { useMemo, useState } from "react";
import { icons } from "@flame/icons/catalog";
import * as Drawings from "@flame/icons/react";
import { createTranslator, type ComponentContext } from "@flame/component-sdk";
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
import { en } from "./locales/en";
import { zh } from "./locales/zh";
import { zhTW } from "./locales/zh-TW";
import { ja } from "./locales/ja";
import { ko } from "./locales/ko";
import { es } from "./locales/es";
import { fr } from "./locales/fr";
import { de } from "./locales/de";

const CATALOGS = { en, zh, "zh-TW": zhTW, ja, ko, es, fr, de };

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

export function Gallery({ context }: { context: ComponentContext }) {
  const t = useMemo(
    () => createTranslator(CATALOGS, context.locale),
    [context.locale],
  );
  const [filter, setFilter] = useState("");
  const [group, setGroup] = useState("");
  const query = filter.trim().toLocaleLowerCase();
  const visible = catalog.filter(
    (icon) => (!group || icon.group === group) && icon.search.includes(query),
  );
  return (
    <Panel
      label={t("panel.title")}
      header={
        <PanelHeader
          title={t("panel.title")}
          description={t("panel.description")}
        />
      }
      controls={
        <FilterBar
          label={t("filters.label")}
          search={
            <SearchField
              size="lg"
              aria-label={t("filter.label")}
              placeholder={t("filter.placeholder")}
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              onClear={() => setFilter("")}
              clearLabel={t("filter.clear")}
            />
          }
        >
          <Select
            size="lg"
            label={t("category.label")}
            value={group}
            onValueChange={setGroup}
            options={[
              { value: "", label: t("category.all") },
              ...groups.map((value) => ({ value, label: value })),
            ]}
          />
        </FilterBar>
      }
    >
      <Metadata role="status">
        {t("counts", { visible: visible.length, total: catalog.length })}
      </Metadata>
      {groups.map((category) => {
        const items = visible.filter((icon) => icon.group === category);
        if (!items.length) return null;
        return (
          <PanelSection
            key={category}
            title={category}
            description={t("section.count", { n: items.length })}
          >
            <TileGrid aria-label={t("section.aria", { category })}>
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
        <EmptyState title={t("empty.title")} sub={t("empty.sub")} />
      )}
    </Panel>
  );
}
