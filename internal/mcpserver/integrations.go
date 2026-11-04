package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addIntegrationTools(server *mcp.Server, backend *Backend) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "authrail_get_sdk_integrations",
		Description: "Discover released Node.js, Go and Python Runtime SDKs, exact installation commands, API entry points and integration responsibilities. Choose the application's language; use authorised local coding tools for installation and wrapping its real function. This tool does not install software, reveal credentials or protect other MCP tools automatically.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noArguments) (*mcp.CallToolResult, map[string]any, error) {
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedRead(ctx, token, "authrail_get_sdk_integrations", "/organisation/mcp/integrations/sdks", Scope, identity)
		return nil, data, err
	})
}
