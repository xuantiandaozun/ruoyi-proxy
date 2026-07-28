package bootstrap

import (
	"encoding/json"
	"fmt"
)

// RefreshAndSyncSpokeProfile 刷新节点运行指标并同步到 Hub，不触发交互式引导。
func RefreshAndSyncSpokeProfile() error {
	profile, err := loadLocalSpokeProfile()
	if err != nil {
		return fmt.Errorf("读取本地 Spoke 档案: %v", err)
	}
	profile.SchemaVersion = currentSpokeProfileVersion
	refreshRuntimeProfile(&profile)
	raw, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 Spoke 档案: %v", err)
	}
	if err := SaveSpokeProfile(raw); err != nil {
		return fmt.Errorf("保存 Spoke 档案: %v", err)
	}
	if err := SyncProfileToHub(profile); err != nil {
		return fmt.Errorf("同步 Spoke 档案: %v", err)
	}
	return nil
}
