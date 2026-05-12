package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DriftRecord describes an inconsistency found and repaired.
type DriftRecord struct {
	Type    string // "wip_stale", "wip_orphan"
	Task    string
	Fixed   bool
	Detail  string
}

// WIPData is the on-disk WIP state.
type WIPData struct {
	Task      string `json:"task"`
	StartedAt string `json:"started_at"`
}

// Reconcile checks and repairs inconsistencies between wip.json and ROADMAP.md.
// Returns the list of drifts found and fixed.
func Reconcile(projectRoot string) ([]DriftRecord, error) {
	var drifts []DriftRecord

	wip := loadWIP(projectRoot)
	if wip.Task == "" {
		return nil, nil // no WIP, nothing to reconcile
	}

	// Check if WIP task is already marked [x] in ROADMAP
	done, err := isTaskDone(projectRoot, wip.Task)
	if err != nil {
		return nil, err
	}

	if done {
		// Drift: WIP says in-progress, ROADMAP says done
		drifts = append(drifts, DriftRecord{
			Type:   "wip_stale",
			Task:   wip.Task,
			Fixed:  true,
			Detail: "Task marked [x] in ROADMAP, clearing stale WIP",
		})
		clearWIP(projectRoot)
	}

	// Check if WIP task still exists in ROADMAP at all
	exists, _ := taskExists(projectRoot, wip.Task)
	if !exists {
		drifts = append(drifts, DriftRecord{
			Type:   "wip_orphan",
			Task:   wip.Task,
			Fixed:  true,
			Detail: "Task not found in ROADMAP, clearing orphan WIP",
		})
		clearWIP(projectRoot)
	}

	return drifts, nil
}

func loadWIP(dir string) WIPData {
	data, err := os.ReadFile(filepath.Join(dir, ".rgt-gsd", "wip.json"))
	if err != nil {
		return WIPData{}
	}
	var w WIPData
	json.Unmarshal(data, &w)
	return w
}

func clearWIP(dir string) {
	os.Remove(filepath.Join(dir, ".rgt-gsd", "wip.json"))
}

func isTaskDone(dir, taskName string) (bool, error) {
	content, err := readRoadmap(dir)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			name := strings.TrimPrefix(trimmed, "- [x]")
			name = strings.TrimPrefix(name, "- [X]")
			name = strings.TrimSpace(name)
			cleanName := strings.ReplaceAll(name, "`", "")
			cleanTask := strings.ReplaceAll(taskName, "`", "")
			if strings.Contains(cleanName, cleanTask) || strings.Contains(cleanTask, cleanName) {
				return true, nil
			}
		}
	}
	return false, nil
}

func taskExists(dir, taskName string) (bool, error) {
	content, err := readRoadmap(dir)
	if err != nil {
		return false, err
	}
	cleanTask := strings.ReplaceAll(taskName, "`", "")
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [") {
			name := strings.TrimPrefix(trimmed, "- [ ]")
			name = strings.TrimPrefix(name, "- [x]")
			name = strings.TrimPrefix(name, "- [X]")
			name = strings.TrimSpace(name)
			cleanName := strings.ReplaceAll(name, "`", "")
			if strings.Contains(cleanName, cleanTask) || strings.Contains(cleanTask, cleanName) {
				return true, nil
			}
		}
	}
	return false, nil
}

func readRoadmap(dir string) (string, error) {
	// Try ROADMAP/ directory first
	dirPath := filepath.Join(dir, "ROADMAP")
	if entries, err := os.ReadDir(dirPath); err == nil && len(entries) > 0 {
		var parts []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				data, err := os.ReadFile(filepath.Join(dirPath, e.Name()))
				if err == nil {
					parts = append(parts, string(data))
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n"), nil
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "ROADMAP.md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
