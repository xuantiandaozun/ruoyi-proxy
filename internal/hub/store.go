package hub

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

const spokesFile = "configs/hub_spokes.json"
const pendingTokenFile = "configs/hub_pending_token.json"

type pendingTokenRecord struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SpokeRecord 已注册 spoke 节点
type SpokeRecord struct {
	ID                  string        `json:"id"`
	TokenHash           string        `json:"token_hash"`
	CreatedAt           time.Time     `json:"created_at"`
	LastSeen            time.Time     `json:"last_seen"`
	Revoked             bool          `json:"revoked"`
	Alias               string        `json:"alias,omitempty"`
	Tags                []string      `json:"tags,omitempty"`
	Group               string        `json:"group,omitempty"`
	Environment         string        `json:"environment,omitempty"`
	Owner               string        `json:"owner,omitempty"`
	Maintenance         bool          `json:"maintenance,omitempty"`
	AllowedCapabilities []string      `json:"allowed_capabilities,omitempty"`
	Profile             *SpokeProfile `json:"profile,omitempty"`
}

type spokeStore struct {
	mu            sync.RWMutex
	spokes        map[string]*SpokeRecord
	pendingTokens map[string]time.Time
}

var defaultStore = &spokeStore{
	spokes:        make(map[string]*SpokeRecord),
	pendingTokens: make(map[string]time.Time),
}

// LoadSpokes 从磁盘加载 spoke 注册表
func LoadSpokes() error {
	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()

	data, err := os.ReadFile(spokesFile)
	if err != nil {
		if os.IsNotExist(err) {
			defaultStore.spokes = make(map[string]*SpokeRecord)
			return nil
		}
		return fmt.Errorf("读取 spoke 配置失败: %v", err)
	}
	var items []SpokeRecord
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("解析 spoke 配置失败: %v", err)
	}
	defaultStore.spokes = make(map[string]*SpokeRecord, len(items))
	for i := range items {
		rec := items[i]
		defaultStore.spokes[rec.ID] = &rec
	}
	return nil
}

func saveSpokesLocked() error {
	items := make([]SpokeRecord, 0, len(defaultStore.spokes))
	for _, rec := range defaultStore.spokes {
		items = append(items, *rec)
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll("configs", 0755); err != nil {
		return err
	}
	return os.WriteFile(spokesFile, data, 0644)
}

// GenerateRegisterToken 生成一次性注册 Token（15 分钟有效，写入磁盘供 CLI/代理共享）
func GenerateRegisterToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	exp := time.Now().Add(15 * time.Minute)
	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()
	defaultStore.mergePendingTokensLocked()
	defaultStore.pendingTokens[token] = exp
	if err := savePendingTokensLocked(); err != nil {
		return "", err
	}
	return token, nil
}

func savePendingTokensLocked() error {
	items := make([]pendingTokenRecord, 0, len(defaultStore.pendingTokens))
	now := time.Now()
	for token, exp := range defaultStore.pendingTokens {
		if now.Before(exp) {
			items = append(items, pendingTokenRecord{Token: token, ExpiresAt: exp})
		}
	}
	if len(items) == 0 {
		if err := os.Remove(pendingTokenFile); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll("configs", 0755); err != nil {
		return err
	}
	return os.WriteFile(pendingTokenFile, data, 0600)
}

func loadPendingTokensLocked() map[string]time.Time {
	items := make(map[string]time.Time)
	data, err := os.ReadFile(pendingTokenFile)
	if err != nil {
		return items
	}
	var records []pendingTokenRecord
	if err := json.Unmarshal(data, &records); err != nil {
		// 兼容旧版单 Token 文件。
		var legacy pendingTokenRecord
		if json.Unmarshal(data, &legacy) != nil {
			return items
		}
		records = []pendingTokenRecord{legacy}
	}
	now := time.Now()
	for _, rec := range records {
		if rec.Token != "" && now.Before(rec.ExpiresAt) {
			items[rec.Token] = rec.ExpiresAt
		}
	}
	return items
}

func (s *spokeStore) mergePendingTokensLocked() {
	if s.pendingTokens == nil {
		s.pendingTokens = make(map[string]time.Time)
	}
	for token, exp := range loadPendingTokensLocked() {
		s.pendingTokens[token] = exp
	}
	now := time.Now()
	for token, exp := range s.pendingTokens {
		if !now.Before(exp) {
			delete(s.pendingTokens, token)
		}
	}
}

func consumeRegisterToken(token string) bool {
	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()
	defaultStore.mergePendingTokensLocked()
	if _, ok := defaultStore.pendingTokens[token]; ok {
		delete(defaultStore.pendingTokens, token)
		_ = savePendingTokensLocked()
		return true
	}
	return false
}

// RegisterSpoke 校验一次性 Token 并颁发长期凭证
func RegisterSpoke(oneTimeToken string) (spokeID, secret string, err error) {
	if !consumeRegisterToken(oneTimeToken) {
		return "", "", fmt.Errorf("注册 Token 无效或已过期")
	}
	secret, err = randomHex(32)
	if err != nil {
		return "", "", err
	}
	spokeID = "spoke-" + secret[:8]
	hash := hashToken(secret)

	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()
	now := time.Now()
	defaultStore.spokes[spokeID] = &SpokeRecord{
		ID:        spokeID,
		TokenHash: hash,
		CreatedAt: now,
		LastSeen:  now,
	}
	if err := saveSpokesLocked(); err != nil {
		return "", "", err
	}
	return spokeID, secret, nil
}

// ValidateSpokeToken 校验 spoke 长期凭证
func ValidateSpokeToken(secret string) (string, bool) {
	hash := hashToken(secret)
	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()
	for id, rec := range defaultStore.spokes {
		if rec.Revoked {
			continue
		}
		if rec.TokenHash == hash {
			now := time.Now()
			persist := now.Sub(rec.LastSeen) >= 30*time.Second
			rec.LastSeen = now
			if persist {
				_ = saveSpokesLocked()
			}
			return id, true
		}
	}
	return "", false
}

// ListSpokes 返回所有 spoke（副本）
func ListSpokes() []SpokeRecord {
	defaultStore.mu.RLock()
	defer defaultStore.mu.RUnlock()
	out := make([]SpokeRecord, 0, len(defaultStore.spokes))
	for _, rec := range defaultStore.spokes {
		out = append(out, cloneSpokeRecord(rec))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GetSpoke 返回指定 spoke 的副本
func GetSpoke(spokeID string) (SpokeRecord, bool) {
	defaultStore.mu.RLock()
	defer defaultStore.mu.RUnlock()
	rec, ok := defaultStore.spokes[spokeID]
	if !ok {
		return SpokeRecord{}, false
	}
	return cloneSpokeRecord(rec), true
}

// UpdateSpokeProfile 更新 spoke 节点档案（Hub 集中管理）
func UpdateSpokeProfile(spokeID string, profile SpokeProfile) error {
	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()
	rec, ok := defaultStore.spokes[spokeID]
	if !ok {
		return fmt.Errorf("spoke 不存在: %s", spokeID)
	}
	if rec.Revoked {
		return fmt.Errorf("spoke 已吊销: %s", spokeID)
	}
	profile.UpdatedAt = time.Now()
	rec.Profile = &profile
	rec.LastSeen = time.Now()
	return saveSpokesLocked()
}

// RevokeSpoke 吊销指定 spoke
func RevokeSpoke(spokeID string) error {
	defaultStore.mu.Lock()
	defer defaultStore.mu.Unlock()
	rec, ok := defaultStore.spokes[spokeID]
	if !ok {
		return fmt.Errorf("spoke 不存在: %s", spokeID)
	}
	rec.Revoked = true
	return saveSpokesLocked()
}

func hashToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
