package agent

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// mdStream 实现"打字机 + 逐段渲染"：
// 每个字符实时打印（打字机效果），遇到段落边界时上移光标覆写为渲染后的 Markdown。
type mdStream struct {
	rawBuf    strings.Builder
	termWidth int
	lines     int // 当前段已完整打印的行数（每次 \n 加 1）
	col       int // 当前列的可见宽度
}

func newMDStream() *mdStream {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		w = 100
	}
	return &mdStream{termWidth: w}
}

func (m *mdStream) feed(text string) {
	for _, r := range text {
		m.rawBuf.WriteRune(r)

		if r == '\n' {
			fmt.Print("\n")
			m.lines++
			m.col = 0

			raw := m.rawBuf.String()
			if strings.HasSuffix(raw, "\n\n") && !m.insideCodeBlock(raw) {
				m.flushParagraph()
			}
		} else {
			fmt.Printf("%c", r) // 打字机
			m.col += runeWidth(r)
			if m.col >= m.termWidth {
				m.lines++
				m.col = 0
			}
		}
	}
}

// finish 将缓冲区剩余内容渲染输出
func (m *mdStream) finish() {
	if strings.TrimSpace(m.rawBuf.String()) != "" {
		m.flushParagraph()
	}
}

// flushParagraph 将光标移回当前段落起始位置，用渲染后的 Markdown 覆写原始文本。
//
// 光标移动规则：
//   - col > 0：光标在某行中间 → 先上移 lines 行，再回行首（\r）
//   - col == 0：光标已在行首（紧跟 \n 之后）→ 直接上移 lines 行
func (m *mdStream) flushParagraph() {
	chunk := strings.TrimSpace(m.rawBuf.String())
	m.rawBuf.Reset()

	if chunk == "" {
		m.lines = 0
		m.col = 0
		return
	}

	if m.col > 0 {
		if m.lines > 0 {
			fmt.Printf("\033[%dA", m.lines) // 上移到第一行
		}
		fmt.Print("\r") // 回到行首
	} else if m.lines > 0 {
		fmt.Printf("\033[%dA", m.lines) // 已在行首，直接上移
	}
	fmt.Print("\033[J") // 清除到屏幕末尾

	rendered := strings.Trim(renderMarkdown(chunk), "\n")
	fmt.Print(rendered)
	fmt.Print("\n\n")

	m.lines = 0
	m.col = 0
}

// insideCodeBlock 判断当前缓冲区是否处于未闭合的代码块内
func (m *mdStream) insideCodeBlock(s string) bool {
	count := 0
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			count++
		}
	}
	return count%2 != 0
}

// runeWidth 返回字符的终端显示宽度（ASCII=1，CJK/全角=2）
func runeWidth(r rune) int {
	if r > 0x7F {
		return 2
	}
	return 1
}
