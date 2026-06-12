package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/officecli/officecli/engine"
)

type ConsolePrompter struct {
	in  io.Reader
	out io.Writer
}

func NewConsolePrompter(in io.Reader, out io.Writer) *ConsolePrompter {
	return &ConsolePrompter{in: in, out: out}
}

func (p *ConsolePrompter) Ask(opts AskOptions) (string, string, error) {
	if _, err := fmt.Fprintln(p.out, opts.Question); err != nil {
		return "", "", err
	}
	for idx, option := range opts.Options {
		if _, err := fmt.Fprintf(p.out, "%d. %s\n", idx+1, option); err != nil {
			return "", "", err
		}
	}
	if opts.AllowFreeform {
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
	if err == nil && index >= 1 && index <= len(opts.Options) {
		return line, opts.Options[index-1], nil
	}
	if !opts.AllowFreeform {
		return "", "", fmt.Errorf("invalid option: %s", line)
	}
	return "", line, nil
}

func (p *ConsolePrompter) ReviewPlan(session *engine.PlanSession) (PlanReviewResponse, error) {
	if session == nil {
		return PlanReviewResponse{}, fmt.Errorf("plan is unavailable")
	}
	if _, err := fmt.Fprintln(p.out, strings.TrimSpace(session.PlanMarkdown)); err != nil {
		return PlanReviewResponse{}, err
	}
	if _, err := fmt.Fprintln(p.out, "Approve this plan? Enter 1 to approve, or type revision instructions:"); err != nil {
		return PlanReviewResponse{}, err
	}
	if _, err := fmt.Fprintln(p.out, "1. Approve plan"); err != nil {
		return PlanReviewResponse{}, err
	}
	reader := bufio.NewReader(p.in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return PlanReviewResponse{}, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return PlanReviewResponse{}, fmt.Errorf("plan review response is required")
	}
	if line == "1" || strings.EqualFold(line, string(PlanReviewApprove)) {
		return PlanReviewResponse{Action: PlanReviewApprove}, nil
	}
	return PlanReviewResponse{Action: PlanReviewRevise, Instruction: line}, nil
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
