import { createSingletonPort } from "@/lib/ports/singletonPort";

interface WorkingDirectoryPicker {
  open(): void;
}

const port = createSingletonPort<WorkingDirectoryPicker>(
  "Working directory picker is not configured",
);

export const configureWorkingDirectoryPicker = port.configure;
export const workingDirectoryPicker = port.get;
