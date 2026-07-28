package hub

import (
	"strings"
	"time"
)

// ControlJobEvent 是随任务持久化、只追加的状态审计事件。
type ControlJobEvent struct {
	Sequence int       `json:"sequence"`
	Type     string    `json:"type"`
	From     string    `json:"from,omitempty"`
	To       string    `json:"to,omitempty"`
	Actor    string    `json:"actor,omitempty"`
	Source   string    `json:"source,omitempty"`
	Summary  string    `json:"summary,omitempty"`
	At       time.Time `json:"at"`
}

func appendControlEvent(job *ControlJob, eventType, from, to, actor, source, summary string, at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}
	job.Events = append(job.Events, ControlJobEvent{
		Sequence: len(job.Events) + 1,
		Type:     strings.TrimSpace(eventType),
		From:     from,
		To:       to,
		Actor:    strings.TrimSpace(actor),
		Source:   strings.TrimSpace(source),
		Summary:  truncateControlText(strings.TrimSpace(summary), 500),
		At:       at,
	})
}

func cloneControlJob(job *ControlJob) ControlJob {
	if job == nil {
		return ControlJob{}
	}
	cloned := *job
	cloned.Action = cloneControlAction(job.Action)
	if job.Events != nil {
		cloned.Events = append([]ControlJobEvent(nil), job.Events...)
	}
	return cloned
}
