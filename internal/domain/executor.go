package domain

import "fmt"

// Executor identifies whose work a task records. Empty legacy values are human.
type Executor string

const (
	ExecutorHuman Executor = "human"
	ExecutorAgent Executor = "agent"
)

func ParseExecutor(s string) (Executor, error) {
	switch Executor(s) {
	case "", ExecutorHuman:
		return ExecutorHuman, nil
	case ExecutorAgent:
		return ExecutorAgent, nil
	}
	return "", fmt.Errorf("executor 는 human 또는 agent 이어야 함: %q", s)
}

func (e Executor) Effective() Executor {
	if e == "" {
		return ExecutorHuman
	}
	return e
}
