package hub

import (
	"fmt"
	"sort"
	"strings"
)

// SpokeGovernanceUpdate 更新由 Hub 管理、不会被 Spoke 档案覆盖的治理字段。
type SpokeGovernanceUpdate struct {
	Alias               string   `json:"alias,omitempty"`
	Tags                []string `json:"tags,omitempty"`
	Group               string   `json:"group,omitempty"`
	Environment         string   `json:"environment,omitempty"`
	Owner               string   `json:"owner,omitempty"`
	Maintenance         bool     `json:"maintenance"`
	AllowedCapabilities []string `json:"allowed_capabilities,omitempty"`
}

// SpokeGovernancePatch 对 Hub 治理字段执行部分更新。
type SpokeGovernancePatch struct {
	Alias               *string   `json:"alias,omitempty"`
	Tags                *[]string `json:"tags,omitempty"`
	Group               *string   `json:"group,omitempty"`
	Environment         *string   `json:"environment,omitempty"`
	Owner               *string   `json:"owner,omitempty"`
	Maintenance         *bool     `json:"maintenance,omitempty"`
	AllowedCapabilities *[]string `json:"allowed_capabilities,omitempty"`
}

// UpdateSpokeGovernance 更新节点标签、分组、负责人、维护状态和能力白名单。
func UpdateSpokeGovernance(spokeID string, update SpokeGovernanceUpdate) (SpokeRecord, error) {
	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()
	record, ok := defaultStore.spokes[strings.TrimSpace(spokeID)]
	if !ok {
		return SpokeRecord{}, fmt.Errorf("spoke 不存在: %s", spokeID)
	}
	before := cloneSpokeRecord(record)

	record.Alias = strings.TrimSpace(update.Alias)
	record.Tags = normalizeGovernanceValues(update.Tags)
	record.Group = strings.TrimSpace(update.Group)
	record.Environment = strings.TrimSpace(update.Environment)
	record.Owner = strings.TrimSpace(update.Owner)
	record.Maintenance = update.Maintenance
	record.AllowedCapabilities = normalizeGovernanceValues(update.AllowedCapabilities)
	if err := saveSpokesLocked(); err != nil {
		*record = before
		return SpokeRecord{}, fmt.Errorf("保存 Spoke 治理配置失败: %v", err)
	}
	return cloneSpokeRecord(record), nil
}

// PatchSpokeGovernance 部分更新节点治理字段。
func PatchSpokeGovernance(spokeID string, patch SpokeGovernancePatch) (SpokeRecord, error) {
	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()
	record, ok := defaultStore.spokes[strings.TrimSpace(spokeID)]
	if !ok {
		return SpokeRecord{}, fmt.Errorf("spoke 不存在: %s", spokeID)
	}
	before := cloneSpokeRecord(record)

	if patch.Alias != nil {
		record.Alias = strings.TrimSpace(*patch.Alias)
	}
	if patch.Tags != nil {
		record.Tags = normalizeGovernanceValues(*patch.Tags)
	}
	if patch.Group != nil {
		record.Group = strings.TrimSpace(*patch.Group)
	}
	if patch.Environment != nil {
		record.Environment = strings.TrimSpace(*patch.Environment)
	}
	if patch.Owner != nil {
		record.Owner = strings.TrimSpace(*patch.Owner)
	}
	if patch.Maintenance != nil {
		record.Maintenance = *patch.Maintenance
	}
	if patch.AllowedCapabilities != nil {
		record.AllowedCapabilities = normalizeGovernanceValues(*patch.AllowedCapabilities)
	}
	if err := saveSpokesLocked(); err != nil {
		*record = before
		return SpokeRecord{}, fmt.Errorf("保存 Spoke 治理配置失败: %v", err)
	}
	return cloneSpokeRecord(record), nil
}

// ListSpokesByGroup 返回指定分组中的未吊销节点。
func ListSpokesByGroup(group string) []SpokeRecord {
	group = strings.TrimSpace(group)
	defaultStore.mu.RLock()
	defer defaultStore.mu.RUnlock()
	var result []SpokeRecord
	for _, record := range defaultStore.spokes {
		if !record.Revoked && record.Group == group {
			result = append(result, cloneSpokeRecord(record))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// SpokeAllowsCapability 判断节点上报能力与 Hub 白名单是否允许指定动作。
func SpokeAllowsCapability(record SpokeRecord, capability string) bool {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return true
	}
	if record.Profile == nil || !containsGovernanceValue(record.Profile.Capabilities, capability) {
		return false
	}
	if len(record.AllowedCapabilities) == 0 {
		return true
	}
	return containsGovernanceValue(record.AllowedCapabilities, capability)
}

func normalizeGovernanceValues(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func containsGovernanceValue(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func cloneSpokeRecord(record *SpokeRecord) SpokeRecord {
	if record == nil {
		return SpokeRecord{}
	}
	cloned := *record
	cloned.Tags = append([]string(nil), record.Tags...)
	cloned.AllowedCapabilities = append([]string(nil), record.AllowedCapabilities...)
	if record.Profile != nil {
		profile := *record.Profile
		profile.PrivateIPs = append([]string(nil), record.Profile.PrivateIPs...)
		profile.Capabilities = append([]string(nil), record.Profile.Capabilities...)
		profile.Services = append([]SpokeServiceRef(nil), record.Profile.Services...)
		cloned.Profile = &profile
	}
	return cloned
}
