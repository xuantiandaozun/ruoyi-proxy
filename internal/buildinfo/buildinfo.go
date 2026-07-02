package buildinfo

// Profile 构建角色：default | hub | spoke（通过 -ldflags 注入）
var Profile = "default"

// IsHub 是否为 Hub 节点构建
func IsHub() bool {
	return Profile == "hub"
}

// IsSpoke 是否为 Spoke 节点构建
func IsSpoke() bool {
	return Profile == "spoke"
}

// ProfileLabel 返回可读标签
func ProfileLabel() string {
	switch Profile {
	case "hub":
		return "Hub"
	case "spoke":
		return "Spoke"
	default:
		return "Default"
	}
}
