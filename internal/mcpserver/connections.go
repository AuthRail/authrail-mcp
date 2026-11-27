package mcpserver

const ConnectionsManageScope = "connections.manage"

// Shared pagination and identity contracts for business accounts, Custom MCP and A2A.
type connectionListInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"Page size from 1 to 10; defaults to 10"`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque next cursor from the previous account page"`
}
type connectionStatusInput struct {
	ConnectionID string `json:"connectionId" jsonschema:"Exact saved account ID returned by its business, Custom MCP or A2A account listing"`
}
