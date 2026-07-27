package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/crertel/meshy-cli/internal/api"
	"github.com/crertel/meshy-cli/internal/store"
)

// Printer writes human or JSON output.
type Printer struct {
	JSON bool
	Out  io.Writer
	Err  io.Writer
}

func NewPrinter(jsonMode bool) *Printer {
	return &Printer{JSON: jsonMode, Out: os.Stdout, Err: os.Stderr}
}

// SetOutput redirects printer writers (tests / embedding).
func (p *Printer) SetOutput(out, err io.Writer) {
	if out != nil {
		p.Out = out
	}
	if err != nil {
		p.Err = err
	}
}

func (p *Printer) JSONValue(v any) error {
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (p *Printer) PrintAny(v any) error {
	if p.JSON {
		return p.JSONValue(v)
	}
	switch t := v.(type) {
	case api.Task:
		return p.printTask(t)
	case []api.Task:
		for i, task := range t {
			if i > 0 {
				fmt.Fprintln(p.Out)
			}
			if err := p.printTask(task); err != nil {
				return err
			}
		}
		if len(t) == 0 {
			fmt.Fprintln(p.Out, "(no tasks)")
		}
		return nil
	case map[string]any:
		b, err := json.MarshalIndent(t, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(p.Out, string(b))
		return nil
	case string:
		fmt.Fprintln(p.Out, t)
		return nil
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(p.Out, string(b))
		return nil
	}
}

func (p *Printer) printTask(t api.Task) error {
	id := t.ID()
	status := t.Status()
	fmt.Fprintf(p.Out, "id:       %s\n", id)
	fmt.Fprintf(p.Out, "status:   %s\n", status)
	if t.Progress() > 0 || status == "IN_PROGRESS" {
		fmt.Fprintf(p.Out, "progress: %v\n", t.Progress())
	}
	if msg := t.TaskErrorMessage(); msg != "" {
		fmt.Fprintf(p.Out, "error:    %s\n", msg)
	}
	if assets := t.Assets(); len(assets) > 0 {
		fmt.Fprintln(p.Out, "assets:")
		for _, a := range assets {
			fmt.Fprintf(p.Out, "  %-24s  %-10s  %s\n", a.Name, a.Kind, a.URL)
		}
	}
	// Common extra fields
	for _, key := range []string{"prompt", "mode", "ai_model", "created_at", "finished_at", "expires_at"} {
		if v, ok := t[key]; ok && v != nil {
			fmt.Fprintf(p.Out, "%s: %v\n", key, v)
		}
	}
	return nil
}

func (p *Printer) PrintCreate(id string, extra map[string]any) error {
	if p.JSON {
		m := map[string]any{"result": id, "id": id}
		for k, v := range extra {
			m[k] = v
		}
		return p.JSONValue(m)
	}
	fmt.Fprintf(p.Out, "created task %s\n", id)
	return nil
}

func (p *Printer) PrintSessions(sessions []store.Session) error {
	if p.JSON {
		return p.JSONValue(sessions)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(p.Out, "(no sessions)")
		return nil
	}
	for _, s := range sessions {
		state := "open"
		if s.ClosedAt != nil {
			state = "closed"
		}
		fmt.Fprintf(p.Out, "%s  %s  [%s]\n", s.ID, s.Name, state)
	}
	return nil
}

func (p *Printer) PrintTracked(tasks []store.TrackedTask) error {
	if p.JSON {
		return p.JSONValue(tasks)
	}
	if len(tasks) == 0 {
		fmt.Fprintln(p.Out, "(no tracked tasks)")
		return nil
	}
	for _, t := range tasks {
		tags := ""
		if len(t.Tags) > 0 {
			tags = " tags=" + strings.Join(t.Tags, ",")
		}
		sess := ""
		if t.SessionID != "" {
			sess = " session=" + t.SessionID
		}
		fmt.Fprintf(p.Out, "%s  %s  status=%s%s%s\n", t.ID, t.Resource, t.Status, sess, tags)
	}
	return nil
}

func (p *Printer) PrintSettings(m map[string]string, redact bool) error {
	out := map[string]string{}
	for k, v := range m {
		if redact && (k == "api_key") && v != "" {
			out[k] = redactSecret(v)
		} else {
			out[k] = v
		}
	}
	if p.JSON {
		return p.JSONValue(out)
	}
	if len(out) == 0 {
		fmt.Fprintln(p.Out, "(no settings)")
		return nil
	}
	for k, v := range out {
		fmt.Fprintf(p.Out, "%s=%s\n", k, v)
	}
	return nil
}

func redactSecret(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "…" + s[len(s)-4:]
}
