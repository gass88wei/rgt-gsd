package plan

import (
	"os"
	"path/filepath"
	"strings"
)

// Step is the next actionable step for the user/agent.
type Step struct {
	Type        string // "task", "slice", "milestone"
	Description string
	File        string // source ROADMAP.md path
}

// Status summarizes project progress from ROADMAP.md.
type Status struct {
	Milestones []MilestoneStatus
}

// MilestoneStatus is one milestone's progress.
type MilestoneStatus struct {
	Name    string
	Slices  []SliceStatus
}

// SliceStatus is one slice's progress.
type SliceStatus struct {
	Name  string
	Tasks []TaskStatus
}

// TaskStatus is one task's state.
type TaskStatus struct {
	Name      string
	Completed bool
}

// Plan reads PROJECT.md and ROADMAP.md from projectRoot and provides status.
type Plan interface {
	// Show returns the full project status.
	Show(projectRoot string) (Status, error)

	// Next returns the next unfinished task.
	Next(projectRoot string) (Step, error)

	// MarkDone marks a task as complete in ROADMAP.md.
	// taskName is matched as a prefix.
	MarkDone(projectRoot string, taskName string) error
}

type filePlan struct{}

// New creates a Plan backed by markdown files.
func New() Plan {
	return &filePlan{}
}

func (p *filePlan) Show(projectRoot string) (Status, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, "ROADMAP.md"))
	if err != nil {
		return Status{}, err
	}
	return parseRoadmap(string(data)), nil
}

func (p *filePlan) Next(projectRoot string) (Step, error) {
	data, err := os.ReadFile(filepath.Join(projectRoot, "ROADMAP.md"))
	if err != nil {
		return Step{}, err
	}
	return findNext(string(data)), nil
}

func parseRoadmap(content string) Status {
	var status Status
	var currentMilestone *MilestoneStatus
	var currentSlice *SliceStatus

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "## Milestone") {
			name := strings.TrimPrefix(line, "## Milestone")
			name = strings.TrimSpace(strings.TrimLeft(name, "0123456789.:- "))
			status.Milestones = append(status.Milestones, MilestoneStatus{Name: name})
			currentMilestone = &status.Milestones[len(status.Milestones)-1]
			currentSlice = nil
			continue
		}

		if strings.HasPrefix(line, "### Slice") {
			name := strings.TrimPrefix(line, "### Slice")
			name = strings.TrimSpace(strings.TrimLeft(name, "0123456789.:- "))
			if currentMilestone != nil {
				currentMilestone.Slices = append(currentMilestone.Slices, SliceStatus{Name: name})
				currentSlice = &currentMilestone.Slices[len(currentMilestone.Slices)-1]
			}
			continue
		}

		if strings.HasPrefix(line, "- [") && currentSlice != nil {
			completed := strings.HasPrefix(line, "- [x]") || strings.HasPrefix(line, "- [X]")
			name := strings.TrimPrefix(line, "- [ ]")
			name = strings.TrimPrefix(name, "- [x]")
			name = strings.TrimPrefix(name, "- [X]")
			name = strings.TrimSpace(name)
			if name != "" {
				currentSlice.Tasks = append(currentSlice.Tasks, TaskStatus{
					Name:      name,
					Completed: completed,
				})
			}
		}
	}
	return status
}

func (p *filePlan) MarkDone(projectRoot string, taskName string) error {
	path := filepath.Join(projectRoot, "ROADMAP.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	newContent := markDone(content, taskName)
	if newContent == content {
		return nil // no change — already done
	}
	return os.WriteFile(path, []byte(newContent), 0o644)
}

func markDone(content, taskName string) string {
	var result []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") {
			name := strings.TrimPrefix(trimmed, "- [ ]")
			name = strings.TrimSpace(name)
			if strings.HasPrefix(name, taskName) {
				line = strings.Replace(line, "- [ ]", "- [x]", 1)
			}
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func findNext(content string) Step {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- [ ]") {
			name := strings.TrimPrefix(line, "- [ ]")
			return Step{
				Type:        "task",
				Description: strings.TrimSpace(name),
			}
		}
	}
	return Step{Type: "done", Description: "All tasks completed"}
}
