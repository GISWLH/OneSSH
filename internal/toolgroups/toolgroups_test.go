package toolgroups

import (
	"strings"
	"testing"
)

func TestParseAcceptsTrimmedListAndRejectsUnknownGroup(t *testing.T) {
	disabled, err := Parse(" Memory , search ,")
	if err != nil {
		t.Fatal(err)
	}
	if !disabled[Memory] || !disabled[Search] {
		t.Fatalf("解析结果 = %#v", disabled)
	}
	if disabled.Enabled(Memory) || !disabled.Enabled(Exec) {
		t.Fatalf("启用判定错误: %#v", disabled)
	}
	if names := disabled.Names(); strings.Join(names, ",") != "search,memory" {
		t.Fatalf("禁用分组名 = %v", names)
	}

	// 拼错分组名必须在启动时报错：静默忽略等于运维以为关掉的工具其实还暴露着。
	if _, err := Parse("memory_recall"); err == nil || !strings.Contains(err.Error(), "未知工具组") {
		t.Fatalf("未知分组错误 = %v", err)
	}
}

func TestParseEmptySpecDisablesNothing(t *testing.T) {
	disabled, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled) != 0 {
		t.Fatalf("空配置禁用了分组: %#v", disabled)
	}
	for _, def := range All {
		if !disabled.Enabled(def.Group) {
			t.Fatalf("分组 %s 默认未启用", def.Group)
		}
	}
}
