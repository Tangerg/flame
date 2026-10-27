import {
  Button,
  Checkbox,
  Metadata,
  ScrollRegion,
  TextField,
  Toolbar,
} from "@flame/ui";
import type { Translate } from "@flame/component-sdk";
import {
  useGetFieldValue,
  type ComponentRenderProps,
} from "@openuidev/react-lang";
import type { DisplayProps } from "../contracts/catalog";
import { stateKey } from "../contracts/document";
import { useGenUIInteraction } from "./interaction";

function cellText(value: unknown): string {
  if (value === undefined || value === null) return "";
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}

export function TableView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["Table"]> & { t: Translate }) {
  const read = useGetFieldValue();
  const { update, readInitial } = useGenUIInteraction();
  const fieldValue = (id: string) =>
    read(undefined, stateKey(id)) ?? readInitial(id);
  const query = props.searchState
    ? (fieldValue(props.searchState) as string)
    : "";
  const sort = props.sortState ? (fieldValue(props.sortState) as string) : "";
  const page = props.pageState ? (fieldValue(props.pageState) as number) : 0;
  const selected = props.selectState
    ? (fieldValue(props.selectState) as string[])
    : [];
  const set = update;
  const divider = sort.lastIndexOf(":");
  const sortKey = divider < 0 ? undefined : sort.slice(0, divider);
  const descending = sort.slice(divider + 1) === "desc";
  const normalized = query.toLocaleLowerCase();
  const filtered = props.rows.filter((row) =>
    props.columns.some((column) =>
      cellText(row[column.key]).toLocaleLowerCase().includes(normalized),
    ),
  );
  if (sortKey !== undefined) {
    filtered.sort((left, right) => {
      const a = left[sortKey];
      const b = right[sortKey];
      const order =
        typeof a === "number" && typeof b === "number"
          ? a - b
          : cellText(a).localeCompare(cellText(b), undefined, {
              numeric: true,
            });
      return descending ? -order : order;
    });
  }
  const pageSize = props.pageState
    ? (props.pageSize ?? 10)
    : Math.max(1, filtered.length);
  const pages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const current = Math.min(Math.max(0, page), pages - 1);
  const shown = filtered.slice(current * pageSize, (current + 1) * pageSize);

  return (
    <div style={{ minWidth: 0 }}>
      {props.searchState && (
        <TextField
          type="search"
          aria-label={t("table.search", { table: props.caption })}
          placeholder={t("table.search", { table: props.caption })}
          value={query}
          onChange={(event) => {
            set(props.searchState!, event.currentTarget.value);
            if (props.pageState) set(props.pageState, 0);
          }}
        />
      )}
      <ScrollRegion label={props.caption}>
        <table dir="auto">
          <caption style={{ textAlign: "start" }}>{props.caption}</caption>
          <thead>
            <tr>
              {props.selectState && <th scope="col">{t("table.selection")}</th>}
              {props.columns.map((column) => (
                <th
                  key={column.key}
                  scope="col"
                  aria-sort={
                    sortKey === column.key
                      ? descending
                        ? "descending"
                        : "ascending"
                      : undefined
                  }
                >
                  {props.sortState ? (
                    <Button
                      variant="bare"
                      aria-label={t("table.sort", { column: column.label })}
                      onClick={() => {
                        const next =
                          sortKey === column.key && !descending
                            ? "desc"
                            : "asc";
                        set(props.sortState!, `${column.key}:${next}`);
                      }}
                    >
                      {column.label}
                      {sortKey === column.key && (
                        <span aria-hidden="true">
                          {descending ? " ↓" : " ↑"}
                        </span>
                      )}
                    </Button>
                  ) : (
                    column.label
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {shown.length === 0 ? (
              <tr>
                <td
                  colSpan={props.columns.length + (props.selectState ? 1 : 0)}
                >
                  {t("table.empty")}
                </td>
              </tr>
            ) : (
              shown.map((row) => {
                const id = String(row[props.rowKey]);
                return (
                  <tr
                    key={id}
                    aria-selected={
                      props.selectState ? selected.includes(id) : undefined
                    }
                  >
                    {props.selectState && (
                      <td>
                        <Checkbox
                          label={t("table.select", { row: id })}
                          checked={selected.includes(id)}
                          onCheckedChange={(checked) =>
                            set(
                              props.selectState!,
                              checked
                                ? [...selected, id]
                                : selected.filter((value) => value !== id),
                            )
                          }
                        />
                      </td>
                    )}
                    {props.columns.map((column) => (
                      <td key={column.key}>{cellText(row[column.key])}</td>
                    ))}
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </ScrollRegion>
      {props.pageState && (
        <Toolbar label={t("table.pagination", { table: props.caption })}>
          <Button
            disabled={current === 0}
            onClick={() => set(props.pageState!, current - 1)}
          >
            {t("previous")}
          </Button>
          <Metadata role="status">
            {t("page", { current: current + 1, total: pages })}
          </Metadata>
          <Button
            disabled={current + 1 === pages}
            onClick={() => set(props.pageState!, current + 1)}
          >
            {t("next")}
          </Button>
        </Toolbar>
      )}
    </div>
  );
}
