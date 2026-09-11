package mcpserver

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"onessh/internal/cryptox"
	"onessh/internal/events"
	"onessh/internal/hostmanager"
	"onessh/internal/memoryx"
	"onessh/internal/sshpool"
	"onessh/internal/store"
	"onessh/internal/toolgroups"
)

func newTestServer(t *testing.T, opts Options) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := cryptox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	pool := sshpool.New(st, box)
	t.Cleanup(pool.Close)
	if opts.DataDir == "" {
		opts.DataDir = dir
	}
	server := New(st, pool, events.New(), hostmanager.New(st, box, pool), memoryx.New(st, memoryx.EmbeddingConfig{}), opts)
	t.Cleanup(server.Close)
	return server
}

func connectClient(t *testing.T, ctx context.Context, server *Server) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.MCP.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { serverSession.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "group-test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func listToolNames(t *testing.T, ctx context.Context, session *mcp.ClientSession) []string {
	t.Helper()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

// 禁用一个工具组后，它既不能出现在 tools/list，也不能出现在服务器提示词里：
// 模型调用前只读得到这两处，任何一处留下工具名都会让它反复调用不存在的工具。
func TestDisabledGroupLeavesNoTraceInCatalogOrInstructions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := newTestServer(t, Options{MCPApps: true, DisabledTools: toolgroups.Disabled{toolgroups.Memory: true}})
	session := connectClient(t, ctx, server)

	for _, name := range listToolNames(t, ctx, session) {
		if strings.HasPrefix(name, "memory_") {
			t.Fatalf("记忆工具仍暴露: %s", name)
		}
	}
	instructions := session.InitializeResult().Instructions
	if strings.Contains(instructions, "memory_") || strings.Contains(instructions, "记忆库") {
		t.Fatalf("提示词仍提到记忆: %s", instructions)
	}
	// 只关记忆，其余工具与说明必须原样保留。
	for _, keyword := range []string{"hosts_list", "job_start", "file_edit", "exec_many", "output_read"} {
		if !strings.Contains(instructions, keyword) {
			t.Fatalf("提示词缺少 %q: %s", keyword, instructions)
		}
	}

	// MCP Apps 卡片按工具名发布，禁用的工具不能留下一张调用不到的卡片。
	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var survivors int
	for _, resource := range resources.Resources {
		if strings.Contains(resource.Name, "memory_") {
			t.Fatalf("记忆工具卡片仍发布: %s", resource.URI)
		}
		survivors++
	}
	if survivors == 0 {
		t.Fatal("其余工具的卡片被一并移除")
	}
}

// 分组表是 ONESSH_DISABLED_TOOLS 的取值定义，也是提示词裁剪的依据：
// 新增工具若不归组，就既关不掉也不会被任何说明覆盖。
func TestToolGroupsCoverEveryRegisteredTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := newTestServer(t, Options{SearchHelper: true})
	registered := listToolNames(t, ctx, connectClient(t, ctx, server))

	grouped := map[string]toolgroups.Group{}
	for _, def := range toolgroups.All {
		for _, tool := range def.Tools {
			if previous, duplicated := grouped[tool]; duplicated {
				t.Fatalf("工具 %s 同时属于 %s 与 %s", tool, previous, def.Group)
			}
			grouped[tool] = def.Group
		}
	}
	for _, name := range registered {
		if _, ok := grouped[name]; !ok {
			t.Fatalf("工具 %s 未归入任何工具组", name)
		}
		delete(grouped, name)
	}
	for tool, group := range grouped {
		t.Fatalf("工具组 %s 声明了未注册的工具 %s", group, tool)
	}
}

// 关掉全部分组时服务器仍要能启动并完成 initialize，只是不提供任何工具。
func TestDisablingEveryGroupYieldsEmptyCatalog(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	disabled := toolgroups.Disabled{}
	for _, def := range toolgroups.All {
		disabled[def.Group] = true
	}
	session := connectClient(t, ctx, newTestServer(t, Options{DisabledTools: disabled}))
	if names := listToolNames(t, ctx, session); len(names) != 0 {
		t.Fatalf("仍有工具暴露: %v", names)
	}
}
