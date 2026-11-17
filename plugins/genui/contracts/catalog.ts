import { z } from "zod3";
import { DynamicStringSchema, componentId } from "@a2ui/web_core/v1_0";
import { Catalog, type ComponentApi } from "@a2ui/web_core";
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
  BASIC_FUNCTIONS,
} from "@a2ui/web_core/catalogs/basic/v1";

export const CATALOG_ID = "https://flame.app/a2ui/catalog/v1";
export const PROTOCOL_VERSION = "1.0";
export const DialogApi = {
  name: "Dialog",
  schema: z
    .object({ label: DynamicStringSchema, child: componentId() })
    .strict(),
};
export const componentApis = [
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
  DialogApi,
];
// External navigation is a host action, never an agent-invoked renderer function.
export const functions = BASIC_FUNCTIONS.filter((fn) => fn.name !== "openUrl");
export function createCatalog<T extends ComponentApi>(
  components: readonly T[],
) {
  return new Catalog(CATALOG_ID, PROTOCOL_VERSION, components, functions, {
    instructions:
      "Flame trusted controls. Bind editable values only below /draft. " +
      "Bind operation results below /data/operations. Actions use explicit event context; " +
      "followup requires {message, payload}; resetDraft restores the captured initial draft. " +
      "Use strings for exact identifiers and quantities.",
  });
}
export const catalog = createCatalog(componentApis);
