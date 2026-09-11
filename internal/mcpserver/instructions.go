package mcpserver

import (
	"strings"

	"onessh/internal/toolgroups"
)

// serverInstructions 按启用的工具分组拼装 initialize 返回的服务器级说明。
//
// 提示词必须和 tools/list 严格对齐：模型在调用前只能读到这两处信息，提示词里出现一个
// 未注册的工具名，模型就会反复尝试调用它并把失败当成权限问题。因此每段说明都绑定分组，
// 分组关闭时整段（或对应的分句）一起消失。
func serverInstructions(disabled toolgroups.Disabled) string {
	on := disabled.Enabled
	sections := make([]string, 0, 8)

	opening := "OneSSH 是受主机权限控制的 SSH 运维网关：通过它连接远程主机"
	var caps []string
	if on(toolgroups.Exec) {
		caps = append(caps, "执行命令")
	}
	if on(toolgroups.Files) {
		caps = append(caps, "读写文件")
	}
	if on(toolgroups.Search) {
		caps = append(caps, "搜索内容")
	}
	if on(toolgroups.Jobs) {
		caps = append(caps, "跑后台任务")
	}
	if on(toolgroups.Monitor) {
		caps = append(caps, "看资源指标")
	}
	if len(caps) > 0 {
		opening += "、" + strings.Join(caps, "、")
	}
	if on(toolgroups.Memory) {
		opening += "，并在跨会话的记忆库里积累运维知识"
	}
	sections = append(sections, opening+"。")

	if on(toolgroups.Hosts) {
		sections = append(sections, "【主机与权限】所有工具的 host 参数只接受 hosts_list 返回的名称，任务开始前先调用一次 hosts_list 确认目标主机、标签与在线状态；可按 tags 区分环境或用途后再选主机。名称不在授权列表时会返回 host not authorized。hosts_manage_list、host_create、host_update、host_test、host_reset_fingerprint、host_delete 属于全局配置管理，需要独立的 manage_hosts 权限，且该权限不会扩大命令与文件工具的可访问主机范围。主机可配置 jump_host 跳板，连接自动经跳板隧道建立，对命令与文件工具透明。")
	}

	var specialized []string
	if on(toolgroups.Search) {
		specialized = append(specialized, "搜内容用 grep，找路径用 find")
	}
	if on(toolgroups.Files) {
		specialized = append(specialized, "看目录用 file_list，读文件用 file_read，整文件覆盖用 file_write，局部改写用 file_edit，跨主机复制用 file_transfer")
	}
	if on(toolgroups.Monitor) {
		specialized = append(specialized, "看 CPU/内存/磁盘用 host_status")
	}
	if on(toolgroups.Image) {
		specialized = append(specialized, "看图片用 image_view")
	}
	if on(toolgroups.Exec) && len(specialized) > 0 {
		sections = append(sections, "【优先用专用工具，不要拿 exec 兜底】"+strings.Join(specialized, "，")+
			"。这些工具返回结构化结果，并已处理引号转义、超时、大小上限与输出截断；用 exec 拼 cat/sed/scp/grep 更容易踩到引号与转义问题，只有在没有对应专用工具时才用 exec。")
	}

	var running []string
	if on(toolgroups.Exec) {
		running = append(running, "exec 是同步调用，timeout_s 上限 600 秒，适合秒级到分钟级的命令。")
	}
	if on(toolgroups.Jobs) {
		running = append(running, "预计耗时更久、或希望连接中断后继续运行的命令，用 job_start 起后台任务，再用 job_status 判断是否结束、job_logs 增量看日志、必要时 job_kill 终止。")
	}
	if on(toolgroups.Exec) {
		running = append(running, "exec 的工作目录按 host+session 持久保存，cd 之后下一次 exec 仍在该目录；环境变量用 session_env 设置。互不相干的并行工作请用不同 session 标签，避免彼此改动 cwd。输出被截断时结果里会带 artifact_id，用 output_read 分页或正则过滤，不要重跑命令再加 head/tail。")
	}
	if on(toolgroups.Fanout) {
		running = append(running, "同一条命令要在多台主机上跑用 exec_many。")
	}
	if len(running) > 0 {
		sections = append(sections, "【执行】"+strings.Join(running, ""))
	}

	var care []string
	if on(toolgroups.Exec) || on(toolgroups.Files) {
		care = append(care, "删除、覆盖、重启服务、改防火墙这类破坏性操作前，先读取现状确认目标正确。")
	}
	if on(toolgroups.Files) {
		care = append(care, "file_edit 传上一次 file_read 返回的 expected_sha256 开启乐观锁，避免覆盖他人的并发修改；file_write 是整文件覆盖，改配置优先 file_edit。")
	}
	if on(toolgroups.Exec) {
		care = append(care, "命令里不要内联明文密码或私钥；所有工具调用都会写入网关审计日志。")
	} else {
		care = append(care, "所有工具调用都会写入网关审计日志。")
	}
	sections = append(sections, "【修改远程状态要谨慎】"+strings.Join(care, ""))

	if on(toolgroups.Memory) {
		sections = append(sections,
			"【记忆】执行涉及某台主机的任务前，如果历史部署路径、服务拓扑、故障经验或运维约束可能相关，先调用 memory_recall，并用 host 指定目标主机、用 query 描述当前具体问题；召回内容可能过期，不得替代现场文件读取、命令输出或监控数据的验证。memory_recall 没有结果时继续正常调查，不得把“没有记忆”等同于事实不存在。",
			"确认一个以后仍有价值的事实、解决方案或踩坑记录后，调用 memory_remember 写成简洁、自包含、便于检索的结论，并存入对应主机 bank；只有确实跨主机通用的规则才写入全局 bank。不得保存密码、私钥、访问令牌或其他秘密，不保存瞬时命令输出、未验证猜测或低价值重复信息。推断内容使用 veracity=inferred，工具直接观测的事实使用 veracity=tool，并按长期价值设置 importance。",
			"事实发生变化时优先调用 memory_update 修正原记录；确定记录已经错误、失效或不应保留时才调用 memory_forget。memory_list 与 memory_stats 用于核对某个 bank 的内容与规模。memory_sleep 是按 bank 执行的维护工具，只在需要整理记忆时使用，不必在每次任务中调用。")
	}

	return strings.Join(sections, "\n\n")
}
