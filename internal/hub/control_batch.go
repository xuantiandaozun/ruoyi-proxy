package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const maxControlBatchTargets = 100

var controlBatchMu sync.Mutex

// ControlJobBatch 是一次多节点下发产生的独立任务集合。
type ControlJobBatch struct {
	ID   string       `json:"id"`
	Jobs []ControlJob `json:"jobs"`
}

// EnqueueControlJobBatch 为多个 Spoke 创建参数相同、结果独立的远程任务。
func EnqueueControlJobBatch(spokeIDs []string, command, workDir string, timeoutSecs int, options ControlJobOptions) (ControlJobBatch, error) {
	targets, err := normalizeControlTargets(spokeIDs)
	if err != nil {
		return ControlJobBatch{}, err
	}
	for _, spokeID := range targets {
		spoke, ok := GetSpoke(spokeID)
		if !ok {
			return ControlJobBatch{}, fmt.Errorf("spoke 不存在: %s", spokeID)
		}
		if spoke.Revoked {
			return ControlJobBatch{}, fmt.Errorf("spoke 已吊销: %s", spokeID)
		}
		if spoke.Maintenance {
			return ControlJobBatch{}, fmt.Errorf("spoke 处于维护状态: %s", spokeID)
		}
		if options.Action != nil {
			required := options.Action.RequiredCapability()
			if required != "" && !SpokeAllowsCapability(spoke, required) {
				return ControlJobBatch{}, fmt.Errorf("spoke 缺少或未获准能力 %s: %s", required, spokeID)
			}
		}
	}
	batchID := ""
	if strings.TrimSpace(options.IdempotencyKey) != "" {
		batchID = controlBatchID(options.IdempotencyKey, targets)
	} else {
		generatedID, err := newControlJobID()
		if err != nil {
			return ControlJobBatch{}, err
		}
		batchID = strings.Replace(generatedID, "job-", "batch-", 1)
	}

	controlBatchMu.Lock()
	defer controlBatchMu.Unlock()
	existingIDs, err := controlJobIDs()
	if err != nil {
		return ControlJobBatch{}, err
	}
	batch := ControlJobBatch{ID: batchID, Jobs: make([]ControlJob, 0, len(targets))}
	for _, spokeID := range targets {
		targetOptions := options
		targetOptions.BatchID = batchID
		if options.IdempotencyKey != "" {
			targetOptions.IdempotencyKey = controlBatchIdempotencyKey(options.IdempotencyKey, spokeID)
		}
		job, enqueueErr := EnqueueControlJobWithOptions(spokeID, command, workDir, timeoutSecs, targetOptions)
		if enqueueErr != nil {
			if rollbackErr := rollbackControlBatch(batch.Jobs, existingIDs); rollbackErr != nil {
				return ControlJobBatch{}, fmt.Errorf("创建批量任务失败: %v；回滚失败: %v", enqueueErr, rollbackErr)
			}
			return ControlJobBatch{}, enqueueErr
		}
		batch.Jobs = append(batch.Jobs, job)
	}
	return batch, nil
}

func normalizeControlTargets(spokeIDs []string) ([]string, error) {
	if len(spokeIDs) == 0 {
		return nil, fmt.Errorf("至少需要一个目标 Spoke")
	}
	if len(spokeIDs) > maxControlBatchTargets {
		return nil, fmt.Errorf("单批任务最多支持 %d 个 Spoke", maxControlBatchTargets)
	}
	seen := make(map[string]bool, len(spokeIDs))
	targets := make([]string, 0, len(spokeIDs))
	for _, value := range spokeIDs {
		spokeID := strings.TrimSpace(value)
		if spokeID == "" || seen[spokeID] {
			continue
		}
		seen[spokeID] = true
		targets = append(targets, spokeID)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("目标 Spoke 不能为空")
	}
	sort.Strings(targets)
	return targets, nil
}

func controlBatchID(base string, targets []string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(base) + "\x00" + strings.Join(targets, "\x00")))
	return "batch-" + hex.EncodeToString(sum[:8])
}
func controlBatchIdempotencyKey(base, spokeID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(base) + "\x00" + spokeID))
	return "batch-" + hex.EncodeToString(sum[:16])
}

func controlJobIDs() (map[string]bool, error) {
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(defaultControlStore.jobs))
	for jobID := range defaultControlStore.jobs {
		result[jobID] = true
	}
	return result, nil
}

func rollbackControlBatch(jobs []ControlJob, existingIDs map[string]bool) error {
	if len(jobs) == 0 {
		return nil
	}
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	for _, job := range jobs {
		if !existingIDs[job.ID] {
			delete(defaultControlStore.jobs, job.ID)
		}
	}
	if err := defaultControlStore.saveLocked(); err != nil {
		return fmt.Errorf("保存批量任务回滚结果失败: %v", err)
	}
	return nil
}
