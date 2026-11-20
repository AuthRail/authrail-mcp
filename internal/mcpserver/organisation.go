package mcpserver

import (
	"errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
)

type organisationPersonInput struct {
	RecordID string `json:"recordId"`
	Cursor   string `json:"cursor,omitempty" jsonschema:"Team continuation from this exact person in the connected workspace"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Team page size1 to20; default20"`
}
type organisationSearchInput struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Query  string `json:"query,omitempty" jsonschema:"Optional bounded name/email search"`
}
type organisationPeopleInput struct {
	Cursor       string `json:"cursor,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Query        string `json:"query,omitempty"`
	Status       string `json:"status,omitempty" jsonschema:"active or suspended"`
	DepartmentID string `json:"departmentId,omitempty"`
	TeamID       string `json:"teamId,omitempty"`
}
type organisationTeamsInput struct {
	Cursor       string `json:"cursor,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Query        string `json:"query,omitempty"`
	DepartmentID string `json:"departmentId,omitempty"`
}
type organisationHandoffInput struct {
	Action     string `json:"action" jsonschema:"Named workflow: profile,create_organisation,delete_organisation,restore_organisation,leave_organisation,create_role,update_role,delete_role,update_person,suspend_person,reactivate_person,remove_person,invite_person,revoke_invitation,accept_invitation,create_department,update_department,archive_department,create_team,update_team,archive_team,notification_preferences,identity,logo"`
	ResourceID string `json:"resourceId,omitempty" jsonschema:"Exact existing record UUID for update/delete/suspend/reactivate/remove/revoke/archive workflows"`
}

func orgSearchQuery(input organisationSearchInput) (url.Values, error) {
	if input.Limit > 20 || len(input.Query) > 120 {
		return nil, errors.New("Use at most20 records and120 search characters")
	}
	q := url.Values{}
	if input.Query != "" {
		q.Set("query", input.Query)
	}
	page, err := workspacePage(q, workspacePageInput{Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return nil, err
	}
	return url.ParseQuery(page)
}
func addOrganisationReadTools(server *mcp.Server, backend *Backend) {
	addWorkspaceRead(server, backend, "authrail_get_organisation", "Read the selected workspace profile and your own membership, without private storage references.", OrganisationReadScope, "/organisation/mcp/organisation")
	addWorkspaceRead(server, backend, "authrail_list_organisation_permissions", "Read only your current effective permissions in this workspace. This grants no access.", OrganisationReadScope, "/organisation/mcp/organisation/permissions")
	addWorkspaceRead(server, backend, "authrail_get_organisation_notification_preferences", "Read your own membership's organisation notification preferences.", OrganisationReadScope, "/organisation/mcp/organisation/notification-preferences")
	for _, spec := range []struct{ name, path string }{{"authrail_list_organisation_roles", "roles"}, {"authrail_list_organisation_invitations", "invitations"}, {"authrail_list_organisation_departments", "departments"}} {
		addWorkspaceTool(server, backend, delegatedToolInput[organisationSearchInput]{name: spec.name, description: "List permitted workspace " + spec.path + " with a bounded account/workspace/tool/filter cursor. Existing domain permissions apply.", scope: OrganisationReadScope, method: http.MethodGet, readonly: true, prepare: func(input organisationSearchInput) (string, any, error) {
			q, err := orgSearchQuery(input)
			return "/organisation/mcp/organisation/" + spec.path + "?" + q.Encode(), nil, err
		}})
	}
	for _, spec := range []struct{ name, path string }{{"authrail_list_organisation_domains", "domains"}, {"authrail_list_organisation_sso_connections", "sso-connections"}} {
		addWorkspaceTool(server, backend, delegatedToolInput[workspacePageInput]{name: spec.name, description: "List permitted " + spec.path + " metadata only; identity-management permission is required and secrets remain private.", scope: OrganisationReadScope, method: http.MethodGet, readonly: true, prepare: func(input workspacePageInput) (string, any, error) {
			if input.Limit > 20 {
				return "", nil, errors.New("Use at most20 records")
			}
			q, err := workspacePage(url.Values{}, input)
			return "/organisation/mcp/organisation/" + spec.path + "?" + q, nil, err
		}})
	}
	addWorkspaceTool(server, backend, delegatedToolInput[organisationPeopleInput]{name: "authrail_list_organisation_people", description: "List permitted workspace members with explicit filters and context-bound pagination.", scope: OrganisationReadScope, method: http.MethodGet, readonly: true, prepare: func(input organisationPeopleInput) (string, any, error) {
		q, err := orgSearchQuery(organisationSearchInput{Cursor: input.Cursor, Limit: input.Limit, Query: input.Query})
		if err != nil {
			return "", nil, err
		}
		if input.Status != "" && input.Status != "active" && input.Status != "suspended" {
			return "", nil, errors.New("Use active or suspended status")
		}
		if input.DepartmentID != "" && !frameworkID.MatchString(input.DepartmentID) || input.TeamID != "" && !frameworkID.MatchString(input.TeamID) {
			return "", nil, errors.New("Use exact department/team references")
		}
		if input.Status != "" {
			q.Set("status", input.Status)
		}
		if input.DepartmentID != "" {
			q.Set("departmentId", input.DepartmentID)
		}
		if input.TeamID != "" {
			q.Set("teamId", input.TeamID)
		}
		return "/organisation/mcp/organisation/people?" + q.Encode(), nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[organisationTeamsInput]{name: "authrail_list_organisation_teams", description: "List permitted teams with an optional exact department and bounded cursor.", scope: OrganisationReadScope, method: http.MethodGet, readonly: true, prepare: func(input organisationTeamsInput) (string, any, error) {
		q, err := orgSearchQuery(organisationSearchInput{Cursor: input.Cursor, Limit: input.Limit, Query: input.Query})
		if err != nil {
			return "", nil, err
		}
		if input.DepartmentID != "" {
			if !frameworkID.MatchString(input.DepartmentID) {
				return "", nil, errors.New("Use an exact department reference")
			}
			q.Set("departmentId", input.DepartmentID)
		}
		return "/organisation/mcp/organisation/teams?" + q.Encode(), nil, nil
	}})
	addWorkspaceTool(server, backend, delegatedToolInput[organisationPersonInput]{name: "authrail_get_organisation_person", description: "Read one permitted person and a bounded page of their teams. Continue with this person's exact cursor; no other person's memberships are included.", scope: OrganisationReadScope, method: http.MethodGet, readonly: true, prepare: func(input organisationPersonInput) (string, any, error) {
		if !frameworkID.MatchString(input.RecordID) || input.Limit > 20 {
			return "", nil, errors.New("Choose an exact person and at most20 teams")
		}
		query, err := workspacePage(url.Values{}, workspacePageInput{Cursor: input.Cursor, Limit: input.Limit})
		return "/organisation/mcp/organisation/people/" + input.RecordID + "?" + query, nil, err
	}})

	for _, spec := range []struct{ name, path string }{{"authrail_get_organisation_role", "roles"}, {"authrail_get_organisation_department", "departments"}, {"authrail_get_organisation_team", "teams"}} {
		addWorkspaceTool(server, backend, delegatedToolInput[workspaceRecordInput]{name: spec.name, description: "Read one exact permitted workspace " + spec.path + " record. Record identity never bypasses the current domain permission.", scope: OrganisationReadScope, method: http.MethodGet, readonly: true, prepare: func(input workspaceRecordInput) (string, any, error) {
			if !frameworkID.MatchString(input.RecordID) {
				return "", nil, errors.New("Use an exact record reference from this workspace")
			}
			return "/organisation/mcp/organisation/" + spec.path + "/" + input.RecordID, nil, nil
		}})
	}
}
func addOrganisationManageTools(server *mcp.Server, backend *Backend) {
	addWorkspaceTool(server, backend, delegatedToolInput[organisationHandoffInput]{name: "authrail_manage_organisation", description: "Continue one named organisation workflow in its existing authenticated human screen. No candidate values, invitations, role grants, identity configuration or lifecycle changes are applied by this navigation tool. Private invitation tokens stay in email/browser channels.", scope: OrganisationManageScope, method: http.MethodPost, readonly: true, prepare: func(input organisationHandoffInput) (string, any, error) {
		if input.ResourceID != "" && !frameworkID.MatchString(input.ResourceID) {
			return "", nil, errors.New("Use an exact existing record reference")
		}
		return "/organisation/mcp/organisation/handoff", input, nil
	}})
}
