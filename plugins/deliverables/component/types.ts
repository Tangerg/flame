import type { MethodInputs, MethodOutputs } from "./methods.generated";
import { METHODS } from "../contracts/methods";

export type DocumentRead = MethodOutputs[typeof METHODS.read];
export type Header = MethodOutputs[typeof METHODS.list]["entries"][number];
export type Revision = DocumentRead["revision"];
export type AssessmentHeader = DocumentRead["assessments"][number];
export type Assessment = MethodOutputs[typeof METHODS.assessment];
export type Asset = Revision["native"];
export type Upload = MethodOutputs[typeof METHODS.upload];
export type Provenance = MethodInputs[typeof METHODS.create]["provenance"];
export type Format = Header["format"];
export type OfficeFormat = Exclude<Format, "pdf">;

export const emptyProvenance = (): Provenance => ({
  sources: [],
  assumptions: [],
  units: "",
  reproduction: "",
});
