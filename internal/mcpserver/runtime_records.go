package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const RuntimeReadScope = "runtime.read"

func addRuntimeRecordTools(server *mcp.Server, backend *Backend) {
	mcp.AddTool(server, &mcp.Tool{Name: "authrail_get_runtime_action", Description: "Inspect a retained Action using the deployment and Action IDs from the protected connector. Requires separate Runtime read consent and your AI Inventory visibility. Returns bounded status/certificate/receipt metadata, without private signed payloads or approval review material. Permission, application-reported execution and independently verified provider effects remain distinct; Observe does not prove enforcement.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, _ *mcp.CallToolRequest, input struct {
		DeploymentID string `json:"deploymentId"`
		ActionID     string `json:"actionId"`
	}) (*mcp.CallToolResult, map[string]any, error) {
		if !frameworkID.MatchString(input.DeploymentID) || !frameworkID.MatchString(input.ActionID) {
			return nil, nil, errors.New("Use the exact deployment and Action IDs returned for your request")
		}
		token, identity := requestIdentity(ctx)
		data, err := backend.delegatedRead(ctx, token, "authrail_get_runtime_action", "/organisation/mcp/runtime/deployments/"+input.DeploymentID+"/actions/"+input.ActionID, RuntimeReadScope, identity)
		return nil, data, err
	})
}
