// Package graphql provides a GraphQL HTTP client and MCP tool registration
// for the Unraid GraphQL API escape hatch.
package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const toolNameGraphQLQuery = "graphql_query"

// GraphQLTools returns a slice of tool registrations for the GraphQL escape
// hatch. It exposes a single "graphql_query" tool that allows callers to
// execute arbitrary GraphQL queries against the Unraid API.
func GraphQLTools(client Client, audit *safety.AuditLogger) []tools.Registration {
	return []tools.Registration{
		toolGraphQLQuery(client, audit),
	}
}

// toolGraphQLQuery constructs the graphql_query Registration.
func toolGraphQLQuery(client Client, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool(toolNameGraphQLQuery,
		mcp.WithDescription("Execute an arbitrary GraphQL query against the Unraid API. Use when direct API access is needed beyond the provided tools."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The GraphQL query or mutation string to execute."),
		),
		mcp.WithString("variables",
			mcp.Description("Optional JSON object string of variables to pass with the query."),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		query := req.GetString("query", "")
		variablesStr := req.GetString("variables", "")

		params := map[string]any{
			"query":     query,
			"variables": variablesStr,
		}

		// Parse variables JSON if provided.
		var parsedVars map[string]any
		if variablesStr != "" {
			if err := json.Unmarshal([]byte(variablesStr), &parsedVars); err != nil {
				errMsg := fmt.Sprintf("parse variables JSON: %v", err)
				tools.LogAudit(audit, toolNameGraphQLQuery, params, "error: "+errMsg, start)
				return tools.ErrorResult(errMsg), nil
			}
		}

		// Execute the query.
		data, err := client.Execute(ctx, query, parsedVars)
		if err != nil {
			tools.LogAudit(audit, toolNameGraphQLQuery, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		// Unmarshal the raw JSON bytes into any so tools.JSONResult can
		// pretty-print it with consistent indentation.
		var parsed any
		if err := json.Unmarshal(data, &parsed); err != nil {
			tools.LogAudit(audit, toolNameGraphQLQuery, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolNameGraphQLQuery, params, "ok", start)
		return tools.JSONResult(parsed), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
