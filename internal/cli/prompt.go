package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/officecli/officecli-internal/engine"
)

type ConsolePrompter struct {
	in  io.Reader
	out io.Writer
}

func NewConsolePrompter(in io.Reader, out io.Writer) *ConsolePrompter {
	return &ConsolePrompter{in: in, out: out}
}

func (p *ConsolePrompter) Ask(question string, options []string, allowFreeform bool) (string, string, error) {
	if _, err := fmt.Fprintln(p.out, question); err != nil {
		return "", "", err
	}
	for idx, option := range options {
		if _, err := fmt.Fprintf(p.out, "%d. %s\n", idx+1, option); err != nil {
			return "", "", err
		}
	}
	if allowFreeform {
		if _, err := fmt.Fprintln(p.out, "Enter an option number or type your answer directly:"); err != nil {
			return "", "", err
		}
	} else {
		if _, err := fmt.Fprintln(p.out, "Enter an option number:"); err != nil {
			return "", "", err
		}
	}
	reader := bufio.NewReader(p.in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", fmt.Errorf("answer is required")
	}
	index, err := strconv.Atoi(line)
	if err == nil && index >= 1 && index <= len(options) {
		return line, options[index-1], nil
	}
	if !allowFreeform {
		return "", "", fmt.Errorf("invalid option: %s", line)
	}
	return "", line, nil
}

func optionLabels(question *engine.PlanQuestion) []string {
	if question == nil {
		return nil
	}
	out := make([]string, 0, len(question.Options))
	for _, option := range question.Options {
		label := option.Label
		if option.Description != "" {
			label += " - " + option.Description
		}
		out = append(out, label)
	}
	return out
}
