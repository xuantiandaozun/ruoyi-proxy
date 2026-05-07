package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"
)

const maxSessionListItems = 20

// SessionMeta 会话元数据，用于会话列表与选择。
type SessionMeta struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	MessageCount int    `json:"message_count"`
	UserTurns    int    `json:"user_turns"`
	AutoTitled   bool   `json:"auto_titled"`
}

// SessionStore 管理 Agent 会话列表与消息文件。
type SessionStore struct {
	rootDir   string
	indexPath string
}

type slashCommandItem struct {
	Command     string
	Description string
}

// NewSessionStore 创建会话存储。
func NewSessionStore() (*SessionStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	rootDir := filepath.Join(home, ".ruoyi-proxy", "agent-sessions")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, err
	}
	return &SessionStore{
		rootDir:   rootDir,
		indexPath: filepath.Join(rootDir, "index.json"),
	}, nil
}

// CreateSession 创建新会话并写入初始消息。
func (s *SessionStore) CreateSession(messages []Message) (*SessionMeta, error) {
	now := time.Now()
	meta := &SessionMeta{
		ID:         now.Format("20060102-150405"),
		Title:      now.Format("2006-01-02"),
		CreatedAt:  now.Format(time.RFC3339),
		UpdatedAt:  now.Format(time.RFC3339),
		AutoTitled: false,
	}
	s.fillStats(meta, messages)
	if err := s.SaveSession(meta, messages); err != nil {
		return nil, err
	}
	return meta, nil
}

// SaveSession 保存会话元数据和消息内容。
func (s *SessionStore) SaveSession(meta *SessionMeta, messages []Message) error {
	if meta == nil {
		return fmt.Errorf("session meta 不能为空")
	}
	meta.UpdatedAt = time.Now().Format(time.RFC3339)
	s.fillStats(meta, messages)
	if err := s.saveMessages(meta.ID, messages); err != nil {
		return err
	}
	items, err := s.ListSessions()
	if err != nil {
		return err
	}
	replaced := false
	for i := range items {
		if items[i].ID == meta.ID {
			items[i] = *meta
			replaced = true
			break
		}
	}
	if !replaced {
		items = append(items, *meta)
	}
	s.sortSessions(items)
	return s.saveIndex(items)
}

// LoadSession 加载指定会话。
func (s *SessionStore) LoadSession(id string) (*SessionMeta, []Message, error) {
	items, err := s.ListSessions()
	if err != nil {
		return nil, nil, err
	}
	for i := range items {
		if items[i].ID == id {
			messages, err := s.loadMessages(id)
			if err != nil {
				return nil, nil, err
			}
			meta := items[i]
			return &meta, messages, nil
		}
	}
	return nil, nil, fmt.Errorf("未找到会话: %s", id)
}

// ListSessions 返回排序后的会话列表。
func (s *SessionStore) ListSessions() ([]SessionMeta, error) {
	data, err := os.ReadFile(s.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionMeta{}, nil
		}
		return nil, err
	}
	var items []SessionMeta
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	s.sortSessions(items)
	return items, nil
}

// ResolveSessionID 根据编号或 ID 解析会话。
func (s *SessionStore) ResolveSessionID(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("会话编号或 ID 不能为空")
	}
	items, err := s.ListSessions()
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.ID == ref {
			return item.ID, nil
		}
	}
	idx := parsePositiveInt(ref)
	if idx <= 0 || idx > len(items) {
		return "", fmt.Errorf("未找到会话: %s", ref)
	}
	return items[idx-1].ID, nil
}

func (s *SessionStore) saveMessages(id string, messages []Message) error {
	path := filepath.Join(s.rootDir, id+".json")
	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (s *SessionStore) loadMessages(id string) ([]Message, error) {
	path := filepath.Join(s.rootDir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var messages []Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *SessionStore) saveIndex(items []SessionMeta) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath, data, 0644)
}

func (s *SessionStore) sortSessions(items []SessionMeta) {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
}

func (s *SessionStore) fillStats(meta *SessionMeta, messages []Message) {
	meta.MessageCount = len(messages)
	meta.UserTurns = countRealUserTurns(messages)
}

func formatSessionList(items []SessionMeta) string {
	if len(items) == 0 {
		return "暂无历史会话"
	}
	if len(items) > maxSessionListItems {
		items = items[:maxSessionListItems]
	}
	var sb strings.Builder
	sb.WriteString("历史会话:\n")
	for i, item := range items {
		timeText := item.UpdatedAt
		if t, err := time.Parse(time.RFC3339, item.UpdatedAt); err == nil {
			timeText = t.Format("2006-01-02 15:04")
		}
		sb.WriteString(fmt.Sprintf("%2d. %-18s  %-16s  %2d轮  %s\n",
			i+1, trimRunes(item.Title, 18), timeText, item.UserTurns, item.ID))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func selectSessionInteractive(items []SessionMeta, currentID string, readInput func(string) (string, error)) (string, bool, error) {
	if len(items) == 0 {
		return "", false, nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return selectSessionFallback(items, readInput)
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return selectSessionFallback(items, readInput)
	}
	defer term.Restore(fd, oldState)

	hideCursor := func() { fmt.Print("\033[?25l") }
	showCursor := func() { fmt.Print("\033[?25h") }
	hideCursor()
	defer showCursor()

	_, height, err := term.GetSize(fd)
	if err != nil || height < 8 {
		height = 14
	}
	maxVisible := height - 4
	if maxVisible < 5 {
		maxVisible = 5
	}

	selected := 0
	for i, item := range items {
		if item.ID == currentID {
			selected = i
			break
		}
	}
	start := 0
	renderedLines := 0
	reader := bufio.NewReader(os.Stdin)

	render := func() {
		if renderedLines > 0 {
			fmt.Printf("\033[%dA", renderedLines)
			fmt.Print("\r\033[0J")
		}
		fmt.Print("\r\033[1;33m历史会话（↑/↓ 或 j/k 选择，Enter 加载，Esc 取消）:\033[0m\r\n")
		renderedLines = 1

		if selected < start {
			start = selected
		} else if selected >= start+maxVisible {
			start = selected - maxVisible + 1
		}

		end := start + maxVisible
		if end > len(items) {
			end = len(items)
		}

		for i := start; i < end; i++ {
			prefix := "  "
			line := sessionMenuLine(items[i], items[i].ID == currentID)
			if i == selected {
				prefix = "\033[1;32m> \033[0m"
				line = "\033[1;32m" + line + "\033[0m"
			}
			fmt.Printf("\r%s%s\r\n", prefix, line)
			renderedLines++
		}
	}

	readKey := func() rune {
		b, err := reader.ReadByte()
		if err != nil {
			return 0
		}
		if b == 0x1b {
			next, _ := reader.ReadByte()
			if next == '[' {
				third, _ := reader.ReadByte()
				switch third {
				case 'A':
					return 'U'
				case 'B':
					return 'D'
				}
			}
			return 0x1b
		}
		return rune(b)
	}

	render()
	for {
		switch readKey() {
		case 'U', 'k', 'K':
			if selected > 0 {
				selected--
				render()
			}
		case 'D', 'j', 'J':
			if selected < len(items)-1 {
				selected++
				render()
			}
		case '\r', '\n':
			if renderedLines > 0 {
				fmt.Printf("\033[%dA", renderedLines)
				fmt.Print("\r\033[0J")
			}
			return items[selected].ID, true, nil
		case 0x1b, 3:
			if renderedLines > 0 {
				fmt.Printf("\033[%dA", renderedLines)
				fmt.Print("\r\033[0J")
			}
			return "", false, nil
		}
	}
}

func selectSessionFallback(items []SessionMeta, readInput func(string) (string, error)) (string, bool, error) {
	fmt.Println(formatSessionList(items))
	choice, err := readInput("\033[1;33m选择会话编号或 ID（回车取消）: \033[0m")
	if err != nil || strings.TrimSpace(choice) == "" {
		return "", false, err
	}
	idx := parsePositiveInt(choice)
	if idx > 0 && idx <= len(items) {
		return items[idx-1].ID, true, nil
	}
	return strings.TrimSpace(choice), true, nil
}

func sessionMenuLine(item SessionMeta, current bool) string {
	timeText := item.UpdatedAt
	if t, err := time.Parse(time.RFC3339, item.UpdatedAt); err == nil {
		timeText = t.Format("01-02 15:04")
	}
	mark := ""
	if current {
		mark = " [当前]"
	}
	return fmt.Sprintf("%-18s  %s  %2d轮  %s%s",
		trimRunes(item.Title, 18), timeText, item.UserTurns, item.ID, mark)
}

func selectSlashCommandInteractive(items []slashCommandItem, readInput func(string) (string, error)) (string, bool, error) {
	if len(items) == 0 {
		return "", false, nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return selectSlashCommandFallback(items, readInput)
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return selectSlashCommandFallback(items, readInput)
	}
	defer term.Restore(fd, oldState)

	hideCursor := func() { fmt.Print("\033[?25l") }
	showCursor := func() { fmt.Print("\033[?25h") }
	hideCursor()
	defer showCursor()

	_, height, err := term.GetSize(fd)
	if err != nil || height < 8 {
		height = 12
	}
	maxVisible := height - 4
	if maxVisible < 5 {
		maxVisible = 5
	}

	selected := 0
	start := 0
	renderedLines := 0
	reader := bufio.NewReader(os.Stdin)

	render := func() {
		if renderedLines > 0 {
			fmt.Printf("\033[%dA", renderedLines)
			fmt.Print("\r\033[0J")
		}
		fmt.Print("\r\033[1;33mAgent 命令（↑/↓ 或 j/k 选择，Enter 执行，Esc 取消）:\033[0m\r\n")
		renderedLines = 1
		if selected < start {
			start = selected
		} else if selected >= start+maxVisible {
			start = selected - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(items) {
			end = len(items)
		}
		for i := start; i < end; i++ {
			line := fmt.Sprintf("%-10s  %s", items[i].Command, items[i].Description)
			prefix := "  "
			if i == selected {
				prefix = "\033[1;32m> \033[0m"
				line = "\033[1;32m" + line + "\033[0m"
			}
			fmt.Printf("\r%s%s\r\n", prefix, line)
			renderedLines++
		}
	}

	readKey := func() rune {
		b, err := reader.ReadByte()
		if err != nil {
			return 0
		}
		if b == 0x1b {
			next, _ := reader.ReadByte()
			if next == '[' {
				third, _ := reader.ReadByte()
				switch third {
				case 'A':
					return 'U'
				case 'B':
					return 'D'
				}
			}
			return 0x1b
		}
		return rune(b)
	}

	render()
	for {
		switch readKey() {
		case 'U', 'k', 'K':
			if selected > 0 {
				selected--
				render()
			}
		case 'D', 'j', 'J':
			if selected < len(items)-1 {
				selected++
				render()
			}
		case '\r', '\n':
			if renderedLines > 0 {
				fmt.Printf("\033[%dA", renderedLines)
				fmt.Print("\r\033[0J")
			}
			return items[selected].Command, true, nil
		case 0x1b, 3:
			if renderedLines > 0 {
				fmt.Printf("\033[%dA", renderedLines)
				fmt.Print("\r\033[0J")
			}
			return "", false, nil
		}
	}
}

func selectSlashCommandFallback(items []slashCommandItem, readInput func(string) (string, error)) (string, bool, error) {
	var sb strings.Builder
	sb.WriteString("Agent 命令:\n")
	for i, item := range items {
		sb.WriteString(fmt.Sprintf("%2d. %-10s %s\n", i+1, item.Command, item.Description))
	}
	fmt.Println(strings.TrimRight(sb.String(), "\n"))
	choice, err := readInput("\033[1;33m选择命令编号或直接输入命令（回车取消）: \033[0m")
	if err != nil || strings.TrimSpace(choice) == "" {
		return "", false, err
	}
	idx := parsePositiveInt(choice)
	if idx > 0 && idx <= len(items) {
		return items[idx-1].Command, true, nil
	}
	return strings.TrimSpace(choice), true, nil
}

func countRealUserTurns(messages []Message) int {
	count := 0
	for _, msg := range messages {
		if msg.Role == "user" && strings.TrimSpace(msg.Content) != autoResumePrompt {
			count++
		}
	}
	return count
}

func parsePositiveInt(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func trimRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= limit {
		return string(runes)
	}
	if limit == 1 {
		return string(runes[:1])
	}
	return string(runes[:limit-1]) + "…"
}
