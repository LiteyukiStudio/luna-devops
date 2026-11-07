package builder

import (
	"regexp"
	"strconv"
	"strings"
)

// buildkitStepLine 匹配 BuildKit plain 输出中的 Dockerfile 指令步骤，例如
// `#16 [runner 5/8] COPY ...` 或单阶段的 `#5 [2/4] RUN ...`。
var buildkitStepLine = regexp.MustCompile(`^(#\d+) \[(?:(\S+) )?(\d+)/(\d+)\]`)

// buildkitVertexStatus 匹配同一顶点后续的完成状态行，例如 `#16 DONE 0.9s` 或 `#14 CACHED`。
var buildkitVertexStatus = regexp.MustCompile(`^(#\d+) (DONE|CACHED)\b`)

// StepCounter 统计 BuildKit plain 输出中 Dockerfile 指令步骤的完成数与已知总数。
// 多阶段构建的各阶段在开始执行时才输出步骤，因此总数会随新阶段出现而增加；已完成数不会减少。
type StepCounter struct {
	stageTotals map[string]int
	vertexSteps map[string]string
	completed   map[string]bool
}

func NewStepCounter() *StepCounter {
	return &StepCounter{stageTotals: map[string]int{}, vertexSteps: map[string]string{}, completed: map[string]bool{}}
}

// Observe 处理一行日志，返回计数是否变化。
func (c *StepCounter) Observe(line string) bool {
	value := strings.TrimSpace(strings.TrimRight(line, "\r"))
	beforeCompleted, beforeTotal := c.Counts()
	if match := buildkitStepLine.FindStringSubmatch(value); match != nil {
		vertex, stage := match[1], match[2]
		index, _ := strconv.Atoi(match[3])
		total, _ := strconv.Atoi(match[4])
		if index > 0 && total >= index {
			if total > c.stageTotals[stage] {
				c.stageTotals[stage] = total
			}
			c.vertexSteps[vertex] = stage + " " + match[3]
		}
	} else if match := buildkitVertexStatus.FindStringSubmatch(value); match != nil {
		if step, ok := c.vertexSteps[match[1]]; ok {
			c.completed[step] = true
		}
	}
	afterCompleted, afterTotal := c.Counts()
	return afterCompleted != beforeCompleted || afterTotal != beforeTotal
}

// Counts 返回已完成步骤数与目前已知的步骤总数。
func (c *StepCounter) Counts() (completed int, total int) {
	for _, stageTotal := range c.stageTotals {
		total += stageTotal
	}
	return len(c.completed), total
}
