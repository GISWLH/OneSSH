package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"onessh/internal/store"
	"onessh/internal/toolgroups"
)

func TestTokenDenylistHidesMemoryAndExecFromListAndCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	server := newTestServer(t, Options{MCPApps: true, SearchHelper: true})
	st := server.Store
	token, err := st.CreateToken(ctx, store.TokenCreate{
		Name: "restricted-agent", Hash: store.TokenHash("secret-deny"), AllHosts: true,
		DisabledTools: []string{"memory", "exec"},
	})
	if err != nil {
		t.Fatal(err)
	}

	resolve := func(*http.Request) (string, string) {
		return "https://onessh.example/mcp", "https://onessh.example/.well-known/oauth-protected-resource/mcp"
	}
	httpServer := httptest.NewServer(Handler(st, server, resolve))
	t.Cleanup(httpServer.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "deny-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpServer.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{token: "secret-deny"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })

	for _, name := range listToolNames(t, ctx, session) {
		if strings.HasPrefix(name, "memory_") || name == "exec" || name == "session_env" || name == "output_read" {
			t.Fatalf("令牌 denylist 仍暴露工具: %s", name)
		}
	}
	instructions := session.InitializeResult().Instructions
	if strings.Contains(instructions, "memory_") || strings.Contains(instructions, "记忆库") {
		t.Fatalf("提示词仍提到记忆: %s", instructions)
	}
	if strings.Contains(instructions, "session_env") || strings.Contains(instructions, "output_read") {
		t.Fatalf("提示词仍提到 exec 组工具: %s", instructions)
	}

	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range resources.Resources {
		if strings.Contains(resource.Name, "memory_") || strings.Contains(resource.URI, "/exec?") || strings.Contains(resource.URI, "/session_env") {
			t.Fatalf("禁用工具的卡片仍发布: %s", resource.URI)
		}
	}

	denied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "memory_stats"})
	if err != nil {
		t.Fatal(err)
	}
	if denied == nil || !denied.IsError {
		t.Fatalf("禁用工具调用应返回 IsError，实际 %#v", denied)
	}
	if text := toolText(denied); !strings.Contains(text, "tool not authorized: memory_stats") {
		t.Fatalf("拒绝文案 = %q", text)
	}

	audit, err := st.ListAudit(ctx, nil, nil, nil, nil, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) == 0 || audit[0].OK || audit[0].Tool != "memory_stats" {
		t.Fatalf("拒绝审计 = %#v", audit)
	}
	if !audit[0].TokenID.Valid || audit[0].TokenID.Int64 != token.ID {
		t.Fatalf("审计令牌 ID = %#v", audit[0].TokenID)
	}
	if !audit[0].TokenName.Valid || audit[0].TokenName.String != token.Name {
		t.Fatalf("审计令牌名 = %#v", audit[0].TokenName)
	}
}

func TestTokenDenylistUnionsWithInstanceDisabledTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	server := newTestServer(t, Options{
		MCPApps: true, SearchHelper: true,
		DisabledTools: toolgroups.Disabled{toolgroups.Image: true},
	})
	st := server.Store
	if _, err := st.CreateToken(ctx, store.TokenCreate{
		Name: "union-agent", Hash: store.TokenHash("secret-union"), AllHosts: true,
		DisabledTools: []string{"files"},
	}); err != nil {
		t.Fatal(err)
	}
	resolve := func(*http.Request) (string, string) {
		return "https://onessh.example/mcp", "https://onessh.example/.well-known/oauth-protected-resource/mcp"
	}
	httpServer := httptest.NewServer(Handler(st, server, resolve))
	t.Cleanup(httpServer.Close)

	session, err := mcp.NewClient(&mcp.Implementation{Name: "union-test", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpServer.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{token: "secret-union"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })

	for _, name := range listToolNames(t, ctx, session) {
		if name == "image_view" || strings.HasPrefix(name, "file_") {
			t.Fatalf("并集禁用后仍暴露: %s", name)
		}
	}
}

func toolText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
