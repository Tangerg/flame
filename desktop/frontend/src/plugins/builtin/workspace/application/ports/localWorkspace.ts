import { createSingletonPort } from "@/lib/ports/singletonPort";

interface LocalWorkspace {
  available(): boolean;
  open(cwd: string, path: string): Promise<boolean>;
  reveal(cwd: string, path: string): Promise<boolean>;
}

const port = createSingletonPort<LocalWorkspace>("Local workspace access is not configured");

export const configureLocalWorkspace = port.configure;
export const localWorkspace = port.get;
