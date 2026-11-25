import type { FileSaveResult } from "@flame/component-sdk";
import type {
  METHODS as DECLARATIONS,
  MethodInputs,
  MethodOutputs,
} from "./methods.generated";
import { METHODS } from "../contracts/methods";

export type DocumentRead = MethodOutputs[typeof METHODS.read];
export type Header = MethodOutputs[typeof METHODS.list]["entries"][number];
export type Revision = DocumentRead["revision"];
export type AssessmentHeader = DocumentRead["assessments"][number];
export type Assessment = MethodOutputs[typeof METHODS.assessment];
export type Asset = Revision["native"];
export type Upload = MethodOutputs[typeof METHODS.upload];
export type Provenance = MethodInputs[typeof METHODS.create]["provenance"];

export const emptyProvenance = (): Provenance => ({
  sources: [],
  assumptions: [],
  units: "",
  reproduction: "",
});

export type CommandMethod = {
  [
    K in keyof typeof DECLARATIONS
  ]: (typeof DECLARATIONS)[K]["kind"] extends "command" ? K : never;
}[keyof typeof DECLARATIONS];

export type Notice =
  | {
      kind: "command";
      method: CommandMethod;
      asset?: Asset;
      revision?: Revision;
    }
  | { kind: "error"; text: string }
  | {
      kind: "save";
      result: FileSaveResult;
      filename: string;
    };
