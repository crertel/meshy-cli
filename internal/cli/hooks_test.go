package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crertel/meshy-cli/internal/cli"
)

func TestHookInterpolateAndEnv(t *testing.T) {
	h := cli.HookContext{
		Workflow:  "textured-3d",
		Name:      "minotaur",
		OutDir:    "/tmp/out/minotaur",
		Profile:   "game-hero",
		Session:   "ez-minotaur",
		PreviewID: "prev-1",
		RefineID:  "ref-1",
		TaskID:    "ref-1",
		Manifest:  "/tmp/out/minotaur/manifest.json",
		Status:    "SUCCEEDED",
		Files: map[string]string{
			"glb":        "/tmp/out/minotaur/minotaur-model.glb",
			"base_color": "/tmp/out/minotaur/minotaur-base_color.png",
		},
	}
	got := h.Interpolate("cp {{glb}} {{out}}/copy.glb && echo {{name}} {{task_id}}")
	if !strings.Contains(got, "/tmp/out/minotaur/minotaur-model.glb") ||
		!strings.Contains(got, "minotaur") ||
		!strings.Contains(got, "ref-1") {
		t.Fatalf("interpolate: %q", got)
	}
	got = h.Interpolate("echo $MESHY_GLB ${MESHY_OUT_DIR}")
	if !strings.Contains(got, "/tmp/out/minotaur/minotaur-model.glb") ||
		!strings.Contains(got, "/tmp/out/minotaur") {
		t.Fatalf("env expand: %q", got)
	}
	env := strings.Join(h.Env(), "\n")
	for _, want := range []string{"MESHY_GLB=", "MESHY_OUT_DIR=", "MESHY_TASK_ID=ref-1", "MESHY_FILE_BASE_COLOR="} {
		if !strings.Contains(env, want) {
			t.Errorf("env missing %q in %s", want, env)
		}
	}
}

func TestRunHooksPost(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "hooked.txt")
	h := cli.HookContext{
		Name:   "x",
		OutDir: dir,
		TaskID: "t1",
		Files:  map[string]string{"glb": filepath.Join(dir, "m.glb")},
	}
	// create dummy glb
	if err := os.WriteFile(h.Files["glb"], []byte("G"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := cli.RunHooks([]string{
		`echo "{{task_id}}" > hooked.txt`,
		`test -f "$MESHY_GLB"`,
	}, h, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "t1" {
		t.Fatalf("marker = %q", b)
	}
}

func TestResolveOutDir(t *testing.T) {
	abs, err := cli.ResolveOutDir("", "hero")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(abs, filepath.Join("out", "hero")) {
		t.Fatalf("default out = %q", abs)
	}
	abs2, err := cli.ResolveOutDir("/tmp/custom", "x")
	if err != nil || abs2 != "/tmp/custom" {
		// Abs may clean path
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(abs2, "custom") {
			t.Fatalf("custom = %q", abs2)
		}
	}
}
