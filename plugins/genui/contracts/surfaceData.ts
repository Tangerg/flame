export const PRESENTATION_DATA_FIELD = "data";
export const OPERATION_RESULTS_FIELD = "operations";

const presentationDataPrefix = `/${PRESENTATION_DATA_FIELD}/`;
export const OPERATION_RESULTS_PATH =
  presentationDataPrefix + OPERATION_RESULTS_FIELD;

export function isPresentationUpdate(path: string | undefined): boolean {
  if (!path?.startsWith(presentationDataPrefix)) return false;
  // A2UI ignores empty path segments, including the trailing slash on a root write.
  const field = path
    .slice(presentationDataPrefix.length)
    .split("/")
    .find(Boolean);
  return field !== undefined && field !== OPERATION_RESULTS_FIELD;
}

export function operationResultPath(id: string): string {
  return `${OPERATION_RESULTS_PATH}/${id.replaceAll("~", "~0").replaceAll("/", "~1")}`;
}
