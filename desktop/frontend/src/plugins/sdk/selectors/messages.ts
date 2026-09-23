import { TOOL_ACTION, TOOL_VIEW_OPENER } from "../kernelPoints";
import { lookupExtensionOwner } from "./extensions";

export function lookupToolActionOwner(id: string): string | undefined {
  return lookupExtensionOwner(TOOL_ACTION, id);
}

export function lookupToolViewOpenerOwner(id: string): string | undefined {
  return lookupExtensionOwner(TOOL_VIEW_OPENER, id);
}
