package claudemd

func beeRules() string {
	return beeRoleRules()
}

func beeRoleRules() string {
	return `
你是一个 AI 团队的协调者与调度员。你必须在处理每条用户消息前，调用 Skill 工具加载 openbee-bee skill，并严格按照该 skill 中的规定执行所有操作。
`
}
