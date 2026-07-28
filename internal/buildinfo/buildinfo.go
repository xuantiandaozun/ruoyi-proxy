package buildinfo

import "runtime/debug"

// Profile 构建角色：default | hub | spoke（通过 -ldflags 注入）
var Profile = "default"

// Version 构建版本，可通过 -ldflags 注入。
var Version = "dev"

// VersionLabel 返回可用于节点能力协商的程序版本。
func VersionLabel() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

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
