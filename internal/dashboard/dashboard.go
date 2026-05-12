package dashboard

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
	"github.com/gass88wei/rgt-gsd/internal/plan"
	"github.com/gass88wei/rgt-gsd/internal/state"
)

// Start launches a local dashboard HTTP server.
func Start(projectDir, addr string) error {
	h := &handler{
		projectDir: projectDir,
		aud:        auditor.New(""),
		pln:        plan.New(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.index)
	mux.HandleFunc("/api/status", h.apiStatus)

	fmt.Printf("Dashboard: http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

type handler struct {
	projectDir string
	aud        auditor.Auditor
	pln        plan.Plan
}

func (h *handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	status, _ := h.pln.Show(h.projectDir)
	steps, _ := h.aud.Log(r.Context(), h.projectDir, "")
	entries, _ := state.LoadKnowledge(h.projectDir)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, pageHeader)

	// Stats
	total := 0
	done := 0
	for _, m := range status.Milestones {
		for _, s := range m.Slices {
			for _, t := range s.Tasks {
				total++
				if t.Completed {
					done++
				}
			}
		}
	}
	fmt.Fprintf(w, `<div class="stats"><div class="stat"><span>%d</span> tasks</div><div class="stat"><span>%d</span> done</div><div class="stat"><span>%d</span> steps</div><div class="stat"><span>%d</span> entries</div></div>`,
		total, done, len(steps), len(entries))

	// Progress bars
	for _, m := range status.Milestones {
		fmt.Fprintf(w, "<h3>%s</h3>", m.Name)
		for _, s := range m.Slices {
			done := 0
			for _, t := range s.Tasks {
				if t.Completed {
					done++
				}
			}
			pct := 0
			if len(s.Tasks) > 0 {
				pct = done * 100 / len(s.Tasks)
			}
			fmt.Fprintf(w, `<div class="slice"><span class="slice-name">%s [%d/%d]</span><div class="bar"><div class="fill" style="width:%d%%"></div></div></div>`,
				s.Name, done, len(s.Tasks), pct)
		}
	}

	// Timeline
	fmt.Fprint(w, "<h3>Timeline</h3><div class='timeline'>")
	for i := len(steps) - 1; i >= 0 && i > len(steps)-21; i-- {
		s := steps[i]
		shortHash := s.Hash
		if len(shortHash) > 12 {
			shortHash = shortHash[:12]
		}
		fmt.Fprintf(w, `<div class="event"><span class="hash">%s</span> <span class="tool">%s</span> <span class="time">%s</span></div>`,
			shortHash, s.Cause.ToolName, s.Timestamp.Format("15:04:05"))
	}
	fmt.Fprint(w, "</div>")

	// Knowledge
	if len(entries) > 0 {
		fmt.Fprint(w, "<h3>Knowledge</h3><div class='knowledge'>")
		for _, e := range entries {
			fmt.Fprintf(w, `<div class="entry"><span class="time">%s</span> %s <span class="hash">%s</span></div>`,
				e.Time, escapeHTML(e.Task), e.Hash)
		}
		fmt.Fprint(w, "</div>")
	}

	fmt.Fprint(w, pageFooter)
}

func (h *handler) apiStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	status, _ := h.pln.Show(h.projectDir)
	steps, _ := h.aud.Log(r.Context(), h.projectDir, "")

	total := 0
	done := 0
	for _, m := range status.Milestones {
		for _, s := range m.Slices {
			for _, t := range s.Tasks {
				total++
				if t.Completed {
					done++
				}
			}
		}
	}
	fmt.Fprintf(w, `{"tasks":{"total":%d,"done":%d},"steps":%d}`, total, done, len(steps))
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

const pageHeader = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>rgt-gsd dashboard</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;margin:0;padding:24px;background:#0d1117;color:#c9d1d9}
h3{color:#58a6ff;margin-top:32px}
.stats{display:flex;gap:16px;margin-bottom:24px}
.stat{background:#161b22;padding:16px 24px;border-radius:8px;text-align:center;border:1px solid #30363d}
.stat span{display:block;font-size:24px;font-weight:bold;color:#58a6ff}
.slice{display:flex;align-items:center;gap:12px;margin:8px 0}
.slice-name{min-width:200px;font-size:13px;color:#8b949e}
.bar{flex:1;height:12px;background:#21262d;border-radius:6px;overflow:hidden}
.fill{height:100%;background:linear-gradient(90deg,#238636,#3fb950);border-radius:6px;transition:width .3s}
.timeline{border-left:2px solid #30363d;margin-left:8px;padding-left:16px}
.event{padding:4px 0;font-size:13px}
.hash{color:#58a6ff;font-family:monospace;margin-right:8px}
.tool{color:#8b949e;margin-right:8px}
.time{color:#484f58;font-size:12px}
.knowledge{margin-top:8px}
.entry{padding:4px 0;font-size:13px;border-bottom:1px solid #21262d}
.entry .hash{font-size:11px;color:#30363d}
</style>
<meta http-equiv="refresh" content="10"></head><body>
<h2>rgt-gsd dashboard</h2>`

const pageFooter = `</body></html>`
