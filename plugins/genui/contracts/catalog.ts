import { z } from "zod3";
import { DynamicStringSchema, componentId } from "@a2ui/web_core/v1_0";
import { Catalog, SpecVersion, type ComponentApi } from "@a2ui/web_core";
import {
  TextApi,
  RowApi,
  ColumnApi,
  ListApi,
  CardApi,
  DividerApi,
  ButtonApi,
  TextFieldApi,
  CheckBoxApi,
  TabsApi,
  OpenUrlApi,
  BASIC_FUNCTIONS,
} from "@a2ui/web_core/catalogs/basic/v1";

export const CATALOG_ID = "https://flame.app/a2ui/catalog/v1";
export const A2UI_VERSION = SpecVersion.V1_0;
const DialogApi = {
  name: "Dialog",
  schema: z
    .object({ label: DynamicStringSchema, child: componentId() })
    .strict(),
};
export const trustedComponentApis = {
  text: TextApi,
  row: RowApi,
  column: ColumnApi,
  list: ListApi,
  card: CardApi,
  divider: DividerApi,
  button: ButtonApi,
  textField: TextFieldApi,
  checkbox: CheckBoxApi,
  tabs: TabsApi,
  dialog: DialogApi,
} as const;
// External navigation is a host action, never an agent-invoked renderer function.
const functions = BASIC_FUNCTIONS.filter((fn) => fn.name !== OpenUrlApi.name);
export function createCatalog<T extends ComponentApi>(
  components: readonly T[],
) {
  return new Catalog(CATALOG_ID, A2UI_VERSION, components, functions, {
    instructions:
      "Flame trusted controls. Bind editable values only below /draft. " +
      "Bind operation results below /data/operations. Actions use explicit event context; " +
      "followup requires {message, payload}; resetDraft restores the captured initial draft. " +
      "Use strings for exact identifiers and quantities.",
  });
}
export const catalog = createCatalog(Object.values(trustedComponentApis));
