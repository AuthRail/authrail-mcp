package mcpserver

import (
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
)

type exportHandoffInput struct {
	Action   string `json:"action" jsonschema:"request or download"`
	ExportID string `json:"exportId,omitempty" jsonschema:"Exact own export UUID, required only for download"`
}

func addExportListTool(server *mcp.Server, backend *Backend, organisation bool) {
	name, scope, path := "authrail_list_my_exports", AccountReadScope, "personal"
	if organisation {
		name, scope, path = "authrail_list_organisation_exports", OrganisationReadScope, "organisation"
	}
	addWorkspaceTool(server, backend, delegatedToolInput[workspacePageInput]{name: name, description: "Read bounded own-requested " + path + " export status metadata. Current domain permissions apply; no archive or signed storage URL is returned.", scope: scope, method: http.MethodGet, readonly: true, prepare: func(input workspacePageInput) (string, any, error) {
		q, err := workspacePage(url.Values{}, input)
		return "/organisation/mcp/exports/" + path + "?" + q, nil, err
	}})
}
func addExportHandoffTool(server *mcp.Server, backend *Backend, organisation bool) {
	name, scope, path := "authrail_manage_export", AccountManageScope, "personal"
	if organisation {
		name, scope, path = "authrail_manage_organisation_export", OrganisationManageScope, "organisation"
	}
	addWorkspaceTool(server, backend, delegatedToolInput[exportHandoffInput]{name: name, description: "Continue " + path + " export creation or exact selected download privately in AuthRail. This changes nothing, returns no signed storage capability and does not bypass recent authentication or source-audience checks.", scope: scope, method: http.MethodPost, readonly: true, prepare: func(input exportHandoffInput) (string, any, error) {
		if input.Action != "request" && input.Action != "download" || input.Action == "download" && !frameworkID.MatchString(input.ExportID) || input.Action == "request" && input.ExportID != "" {
			return "", nil, errors.New("Choose request without an export ID, or download with the exact own export ID")
		}
		return "/organisation/mcp/exports/" + path + "/handoff", input, nil
	}})
}
