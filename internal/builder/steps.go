package builder

import (
	"regexp"
	"strconv"
	"strings"
)

// buildkitStepLine 匹配 BuildKit plain 输出中的 Dockerfile 指令步骤，例如
// `#16 [runner 5/8] COPY ...` 或单阶段的 `#5 [2/4] RUN ...`。
var buildkitStepLine = regexp.MustCompile(`^(#\d+) (\[(?:(\S+) )?(\d+)/(\d+)\].*)$`)

// buildkitVertexStatus 匹配同一顶点后续的完成状态行，例如 `#16 DONE 0.9s` 或 `#14 CACHED`。
var buildkitVertexStatus = regexp.MustCompile(`^(#\d+) (DONE|CACHED)\b`)

// StepCounter 统计 BuildKit plain 输出中 Dockerfile 指令步骤的完成数与已知总数。
// 多阶段构建的各阶段在开始执行时才输出步骤，因此总数会随新阶段出现而增加；已完成数不会减少。
type StepCounter struct {
	stageTotals map[string]int
	vertexSteps map[string]string
	completed   map[string]bool
	// started 按开始顺序保存步骤键和对应的指令文本，例如 `[runner 5/8] RUN pnpm build`。
	started []startedStep
}

type startedStep struct {
	key         string
	instruction string
}

func NewStepCounter() *StepCounter {
	return &StepCounter{stageTotals: map[string]int{}, vertexSteps: map[string]string{}, completed: map[string]bool{}}
}

// Observe 处理一行日志，返回计数是否变化。
func (c *StepCounter) Observe(line string) bool {
	value := strings.TrimSpace(strings.TrimRight(line, "\r"))
	beforeCompleted, beforeTotal := c.Counts()
	beforeCurrent := c.Current()
	if match := buildkitStepLine.FindStringSubmatch(value); match != nil {
		vertex, instruction, stage := match[1], match[2], match[3]
		index, _ := strconv.Atoi(match[4])
		total, _ := strconv.Atoi(match[5])
		if index > 0 && total >= index {
			if total > c.stageTotals[stage] {
				c.stageTotals[stage] = total
			}
			key := stage + " " + match[4]
			if _, seen := c.vertexSteps[vertex]; !seen {
				c.started = append(c.started, startedStep{key: key, instruction: instruction})
			}
			c.vertexSteps[vertex] = key
		}
	} else if match := buildkitVertexStatus.FindStringSubmatch(value); match != nil {
		if step, ok := c.vertexSteps[match[1]]; ok {
			c.completed[step] = true
		}
	}
	afterCompleted, afterTotal := c.Counts()
	return afterCompleted != beforeCompleted || afterTotal != beforeTotal || c.Current() != beforeCurrent
}

// Current 返回最近开始且尚未完成的指令；全部完成时返回最后开始的指令，尚无步骤时为空。
func (c *StepCounter) Current() string {
	for i := len(c.started) - 1; i >= 0; i-- {
		if !c.completed[c.started[i].key] {
			return c.started[i].instruction
		}
	}
	if len(c.started) == 0 {
		return ""
	}
	return c.started[len(c.started)-1].instruction
}

// Counts 返回已完成步骤数与目前已知的步骤总数。
func (c *StepCounter) Counts() (completed int, total int) {
	for _, stageTotal := range c.stageTotals {
		total += stageTotal
	}
	return len(c.completed), total
}
