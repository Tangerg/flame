// Package mcp is the MCP-connection infra: it dials configured MCP servers,
// holds their live sessions, lists their tools, and reconnects them; it is the
// external-system adapter the toolset capability adapter builds over. Source
// owners provide configuration policy; Connections owns admission, live state,
// supersession, and session retirement over Scope and the go-sdk client.
//
// A degraded boot is tolerated: a server that can't be reached is recorded
// "failed" and skipped, so one unreachable server never fails startup; only a
// config mistake (duplicate name / invalid entry) is fatal.
package mcp
