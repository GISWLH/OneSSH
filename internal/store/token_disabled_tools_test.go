package store

import (
	"context"
	"testing"
)

func TestTokenDisabledToolsRoundTripAndRejectUnknownViaAPILayer(t *testing.T) {
	ctx := context.Background()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	token, err := st.CreateToken(ctx, TokenCreate{
		Name: "agent", Hash: "hash-1", AllHosts: true, DisabledTools: []string{"memory", "files"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(token.DisabledTools) != 2 {
		t.Fatalf("created disabled_tools = %#v", token.DisabledTools)
	}
	found, _, err := st.FindToken(ctx, "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(found.DisabledTools) != 2 || found.DisabledTools[0] != "memory" && found.DisabledTools[0] != "files" {
		// order follows insertion JSON, not All order — store does not normalize
		got := map[string]bool{}
		for _, name := range found.DisabledTools {
			got[name] = true
		}
		if !got["memory"] || !got["files"] {
			t.Fatalf("find disabled_tools = %#v", found.DisabledTools)
		}
	}

	updated, err := st.UpdateToken(ctx, token.ID, TokenUpdate{
		Name: "agent", AllHosts: true, DisabledTools: []string{"exec"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.DisabledTools) != 1 || updated.DisabledTools[0] != "exec" {
		t.Fatalf("updated = %#v", updated.DisabledTools)
	}
}
