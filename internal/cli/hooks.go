package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// HookContext is exposed to --pre/--post scripts via env and {{placeholders}}.
type HookContext struct {
	Workflow  string
	Name      string
	OutDir    string // absolute when resolved
	Profile   string
	Session   string
	PreviewID string
	RefineID  string
	TaskID    string // primary finished task (refine or image-to-3d)
	Manifest  string
	Status    string
	// Files maps asset name → absolute path (glb, fbx, base_color, …)
	Files map[string]string
	// Extra arbitrary keys for interpolation
	Extra map[string]string
}

// ResolveOutDir returns an absolute output directory path.
func ResolveOutDir(outDir, name string) (string, error) {
	if outDir == "" {
		outDir = filepath.Join("out", name)
	}
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// Env returns MESHY_* environment assignments for subprocesses.
func (h HookContext) Env() []string {
	env := []string{
		"MESHY_WORKFLOW=" + h.Workflow,
		"MESHY_NAME=" + h.Name,
		"MESHY_OUT=" + h.OutDir,
		"MESHY_OUT_DIR=" + h.OutDir,
		"MESHY_PROFILE=" + h.Profile,
		"MESHY_SESSION=" + h.Session,
		"MESHY_PREVIEW_ID=" + h.PreviewID,
		"MESHY_REFINE_ID=" + h.RefineID,
		"MESHY_TASK_ID=" + h.TaskID,
		"MESHY_MANIFEST=" + h.Manifest,
		"MESHY_STATUS=" + h.Status,
	}
	// Primary convenience aliases
	if h.Files != nil {
		if p, ok := h.Files["glb"]; ok {
			env = append(env, "MESHY_GLB="+p, "MESHY_MODEL="+p)
		}
		keys := make([]string, 0, len(h.Files))
		for k := range h.Files {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			envKey := "MESHY_FILE_" + envKeyPart(k)
			env = append(env, envKey+"="+h.Files[k])
			// short form for common names
			env = append(env, "MESHY_"+envKeyPart(k)+"="+h.Files[k])
		}
	}
	for k, v := range h.Extra {
		env = append(env, "MESHY_"+envKeyPart(k)+"="+v)
	}
	return env
}

func envKeyPart(s string) string {
	s = strings.ToUpper(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// Placeholders returns {{key}} → value for simple string interpolation.
func (h HookContext) Placeholders() map[string]string {
	m := map[string]string{
		"workflow":   h.Workflow,
		"name":       h.Name,
		"out":        h.OutDir,
		"out_dir":    h.OutDir,
		"output":     h.OutDir,
		"profile":    h.Profile,
		"session":    h.Session,
		"preview_id": h.PreviewID,
		"refine_id":  h.RefineID,
		"task_id":    h.TaskID,
		"manifest":   h.Manifest,
		"status":     h.Status,
	}
	if h.Files != nil {
		if p, ok := h.Files["glb"]; ok {
			m["glb"] = p
			m["model"] = p
		}
		for k, v := range h.Files {
			m[k] = v
			m["file."+k] = v
		}
	}
	for k, v := range h.Extra {
		m[k] = v
	}
	return m
}

// Interpolate replaces {{key}} and $MESHY_KEY / ${MESHY_KEY} using the context.
func (h HookContext) Interpolate(s string) string {
	ph := h.Placeholders()
	// {{key}}
	for k, v := range ph {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	// $MESHY_* from env map (order: longer keys first to avoid partial replaces — simple scan)
	envMap := map[string]string{}
	for _, e := range h.Env() {
		if i := strings.IndexByte(e, '='); i > 0 {
			envMap[e[:i]] = e[i+1:]
		}
	}
	// ${VAR}
	for k, v := range envMap {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
		s = strings.ReplaceAll(s, "$"+k, v)
	}
	return s
}

// RunHooks executes shell commands with context env and interpolation.
// dir is the working directory (typically the output dir).
func RunHooks(commands []string, h HookContext, dir string, logf func(string, ...any)) error {
	if len(commands) == 0 {
		return nil
	}
	if dir == "" {
		dir = h.OutDir
	}
	baseEnv := append(os.Environ(), h.Env()...)
	for i, raw := range commands {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		line := h.Interpolate(raw)
		if logf != nil {
			logf("hook[%d]: %s", i+1, line)
		}
		cmd := exec.Command("sh", "-c", line)
		cmd.Env = baseEnv
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook %d failed (%q): %w", i+1, line, err)
		}
	}
	return nil
}
