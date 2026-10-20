type MCPOrigin = { type: "user" } | { type: "installation"; installationId: string };

export interface MCPServerID {
  name: string;
  origin: MCPOrigin;
}
