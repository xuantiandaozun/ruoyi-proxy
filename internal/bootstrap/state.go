package bootstrap

import (
	"encoding/json"
	"os"
)

const stateFile = "configs/bootstrap_state.json"

// State 首次启动自检完成标记
type State struct {
	HubServerDone bool `json:"hub_server_done"`
	HubCLIDone    bool `json:"hub_cli_done"`
	SpokeCLIDone  bool `json:"spoke_cli_done"`
	Version       int  `json:"version"`
}

// LoadState 读取自检状态
func LoadState() State {
	var s State
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

// SaveState 保存自检状态
func SaveState(s State) error {
	if s.Version == 0 {
		s.Version = 1
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll("configs", 0755); err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0644)
}
