package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crertel/meshy-cli/internal/api"
	"github.com/crertel/meshy-cli/internal/cli"
	"github.com/crertel/meshy-cli/internal/store"
)

func TestCLICreateWaitJSONAndTracking(t *testing.T) {
	var gets atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /openapi/v1/image-to-3d", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "cli-task-1"})
	})
	mux.HandleFunc("GET /openapi/v1/image-to-3d/cli-task-1", func(w http.ResponseWriter, r *http.Request) {
		n := gets.Add(1)
		status := "IN_PROGRESS"
		if n >= 2 {
			status = "SUCCEEDED"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       "cli-task-1",
			"status":   status,
			"progress": float64(n * 50),
			"model_urls": map[string]string{
				"glb": "https://cdn.example/model.glb",
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dbPath := filepath.Join(t.TempDir(), "cli.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if _, err := st.CreateSession("sess-1", "batch"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	app := &cli.App{
		Store: st,
		Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard},
	}
	if err := cli.ExecuteWithApp(app, []string{
		"--api-key", "test-key",
		"--base-url", srv.URL,
		"--db", dbPath,
		"--json",
		"--session", "sess-1",
		"--tag", "batch-tag",
		"--poll-interval", "1ms",
		"image-to-3d", "create",
		"--image-url", "https://example.com/a.png",
	}); err != nil {
		t.Fatal(err)
	}

	createJSON := out.String()
	var created map[string]any
	if err := json.Unmarshal([]byte(createJSON), &created); err != nil {
		t.Fatalf("json create: %v out=%q", err, createJSON)
	}
	if created["result"] != "cli-task-1" && created["id"] != "cli-task-1" {
		t.Fatalf("create out = %s", createJSON)
	}

	tasks, err := st.ListTasks(store.TaskFilter{SessionID: "sess-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "cli-task-1" {
		t.Fatalf("tracked = %#v", tasks)
	}
	foundTag := false
	for _, tag := range tasks[0].Tags {
		if tag == "batch-tag" {
			foundTag = true
		}
	}
	if !foundTag {
		t.Fatalf("tags = %#v", tasks[0].Tags)
	}

	out.Reset()
	app2 := &cli.App{
		Store: st,
		Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard},
	}
	if err := cli.ExecuteWithApp(app2, []string{
		"--api-key", "test-key",
		"--base-url", srv.URL,
		"--db", dbPath,
		"--json",
		"--poll-interval", "1ms",
		"image-to-3d", "wait", "cli-task-1",
	}); err != nil {
		t.Fatal(err)
	}
	var waited api.Task
	if err := json.Unmarshal(out.Bytes(), &waited); err != nil {
		t.Fatalf("wait json: %v out=%q", err, out.String())
	}
	if waited.Status() != "SUCCEEDED" {
		t.Fatalf("wait status = %s out=%s", waited.Status(), out.String())
	}
	if waited.ID() != "cli-task-1" {
		t.Fatalf("wait id = %s", waited.ID())
	}
	if path := os.Getenv("EVIDENCE_JSON"); path != "" {
		// Durable proof: real create/wait JSON stdout from shipped CLI path.
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(f, "=== create JSON ===")
		fmt.Fprint(f, createJSON)
		fmt.Fprintln(f, "=== wait JSON ===")
		fmt.Fprint(f, out.String())
		_ = f.Close()
	}
	if lp := os.Getenv("EVIDENCE_LOCAL_STORE"); lp != "" {
		lf, err := os.OpenFile(lp, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(lf, "=== tracked tasks (from TestCLICreateWaitJSONAndTracking) ===")
		tasks, err := st.ListTasks(store.TaskFilter{SessionID: "sess-1"})
		if err != nil {
			t.Fatal(err)
		}
		enc := json.NewEncoder(lf)
		enc.SetIndent("", "  ")
		_ = enc.Encode(tasks)
		_ = lf.Close()
	}
	_ = time.Millisecond
}

func TestCLIConfigRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cfg.db")
	var out bytes.Buffer
	app := &cli.App{Print: &cli.Printer{Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app, []string{
		"--db", dbPath,
		"config", "set", "api_key", "msy_persist",
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "set api_key") {
		t.Fatalf("set out = %q", out.String())
	}

	out.Reset()
	app2 := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app2, []string{
		"--db", dbPath,
		"--json",
		"config", "list",
	}); err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["api_key"] == "msy_persist" {
		t.Fatal("api_key should be redacted in list")
	}
	if m["api_key"] == "" {
		t.Fatal("expected redacted api_key")
	}

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_ = os.Unsetenv("MESHY_API_KEY")
	cfg, src, err := cli.ResolveConfig("", "", dbPath, st)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "msy_persist" || src.APIKey != "sqlite" {
		t.Fatalf("cfg=%q src=%q", cfg.APIKey, src.APIKey)
	}
}

func TestCLISessionCommands(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sess.db")
	var out bytes.Buffer
	app := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app, []string{
		"--db", dbPath,
		"--json",
		"session", "open", "my-work",
	}); err != nil {
		t.Fatal(err)
	}
	var sess store.Session
	if err := json.Unmarshal(out.Bytes(), &sess); err != nil {
		t.Fatalf("session json: %v out=%q", err, out.String())
	}
	if sess.Name != "my-work" || sess.ID == "" {
		t.Fatalf("sess = %#v", sess)
	}

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertTask(store.TrackedTask{
		ID:        "t1",
		Resource:  "text-to-3d",
		SessionID: sess.ID,
		Status:    "PENDING",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddTags("t1", []string{"x"}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	out.Reset()
	app2 := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app2, []string{
		"--db", dbPath,
		"--json",
		"session", "show", sess.ID,
	}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	tasks, ok := payload["tasks"].([]any)
	if !ok || len(tasks) != 1 {
		t.Fatalf("show = %s", out.String())
	}
}

func TestHelpListsResources(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "h.db")
	var buf bytes.Buffer
	root := cli.NewRoot()
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--db", dbPath, "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	help := buf.String()
	for _, want := range []string{"text-to-3d", "image-to-3d", "remesh", "print", "creative-lab", "balance", "session", "config"} {
		if !strings.Contains(help, want) {
			t.Errorf("help missing %q", want)
		}
	}

	buf.Reset()
	root = cli.NewRoot()
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--db", dbPath, "image-to-3d", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	sub := buf.String()
	for _, want := range []string{"create", "get", "list", "delete", "wait", "download", "assets"} {
		if !strings.Contains(sub, want) {
			t.Errorf("image-to-3d help missing %q\n%s", want, sub)
		}
	}
}

func TestAssetsAndDownload(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	assetURL := srv.URL + "/files/image.png"
	mux.HandleFunc("GET /openapi/v1/text-to-image/img-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "img-1",
			"status":     "SUCCEEDED",
			"image_urls": []any{assetURL},
		})
	})
	mux.HandleFunc("GET /files/image.png", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("PNGDATA"))
	})

	dbPath := filepath.Join(t.TempDir(), "a.db")
	outDir := filepath.Join(t.TempDir(), "dl")

	var out bytes.Buffer
	app := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app, []string{
		"--api-key", "k", "--base-url", srv.URL, "--db", dbPath, "--json",
		"text-to-image", "assets", "img-1",
	}); err != nil {
		t.Fatal(err)
	}
	var listed map[string]any
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatalf("assets json: %v %s", err, out.String())
	}
	assets, ok := listed["assets"].([]any)
	if !ok || len(assets) < 1 {
		t.Fatalf("assets payload: %s", out.String())
	}

	out.Reset()
	app2 := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app2, []string{
		"--api-key", "k", "--base-url", srv.URL, "--db", dbPath, "--json",
		"text-to-image", "download", "img-1", "--all", "-o", outDir,
	}); err != nil {
		t.Fatal(err)
	}
	var dl map[string]any
	if err := json.Unmarshal(out.Bytes(), &dl); err != nil {
		t.Fatal(err)
	}
	files, ok := dl["files"].(map[string]any)
	if !ok || len(files) < 1 {
		t.Fatalf("download: %s", out.String())
	}
	for _, p := range files {
		path, _ := p.(string)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "PNGDATA" {
			t.Fatalf("file content %q", b)
		}
	}
}

func TestCreateSyncDownload(t *testing.T) {
	var gets atomic.Int32
	mux := http.NewServeMux()
	var srvURL string
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	mux.HandleFunc("POST /openapi/v1/image-to-3d", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "sync-1"})
	})
	mux.HandleFunc("GET /openapi/v1/image-to-3d/sync-1", func(w http.ResponseWriter, r *http.Request) {
		n := gets.Add(1)
		status := "IN_PROGRESS"
		if n >= 2 {
			status = "SUCCEEDED"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "sync-1",
			"status": status,
			"model_urls": map[string]any{
				"glb": srvURL + "/m.glb",
			},
		})
	})
	mux.HandleFunc("GET /m.glb", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("GLB"))
	})

	dbPath := filepath.Join(t.TempDir(), "s.db")
	outDir := filepath.Join(t.TempDir(), "out")
	var out bytes.Buffer
	app := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app, []string{
		"--api-key", "k", "--base-url", srv.URL, "--db", dbPath, "--json",
		"--poll-interval", "1ms",
		"image-to-3d", "create",
		"--image-url", "https://x/a.png",
		"--sync", "--download", "-o", outDir,
	}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("%v %s", err, out.String())
	}
	files, ok := payload["files"].(map[string]any)
	if !ok || len(files) < 1 {
		t.Fatalf("expected files in %s", out.String())
	}
	_ = srvURL
}

func TestImageModelsCatalog(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "m.db")
	var out bytes.Buffer
	app := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app, []string{
		"--db", dbPath, "--json",
		"text-to-image", "models",
	}); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) < 3 {
		t.Fatalf("models = %s", out.String())
	}
}

func TestEZProfilesAndHelp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "p.db")
	var out bytes.Buffer
	app := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app, []string{"--db", dbPath, "--json", "ez", "profiles"}); err != nil {
		t.Fatal(err)
	}
	var profiles []map[string]any
	if err := json.Unmarshal(out.Bytes(), &profiles); err != nil {
		t.Fatalf("%v %s", err, out.String())
	}
	if len(profiles) < 4 {
		t.Fatalf("profiles = %s", out.String())
	}

	var buf bytes.Buffer
	root := cli.NewRoot()
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--db", dbPath, "ez", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	h := buf.String()
	for _, want := range []string{"textured-3d", "image-to-3d", "retexture", "remesh", "image-to-image", "multi-image-to-3d", "print-analyze", "print-repair", "profiles"} {
		if !strings.Contains(h, want) {
			t.Errorf("ez help missing %q", want)
		}
	}
}

func TestEZTextured3DWorkflow(t *testing.T) {
	var previewGets, refineGets atomic.Int32
	mux := http.NewServeMux()
	var srv *httptest.Server
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("POST /openapi/v2/text-to-3d", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mode, _ := body["mode"].(string)
		if mode == "preview" {
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "prev-1"})
			return
		}
		if mode == "refine" {
			if body["preview_task_id"] != "prev-1" {
				t.Errorf("preview_task_id = %v", body["preview_task_id"])
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "ref-1"})
			return
		}
		http.Error(w, "bad mode", 400)
	})
	mux.HandleFunc("GET /openapi/v2/text-to-3d/prev-1", func(w http.ResponseWriter, r *http.Request) {
		n := previewGets.Add(1)
		st := "IN_PROGRESS"
		if n >= 2 {
			st = "SUCCEEDED"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "prev-1", "status": st, "mode": "preview"})
	})
	mux.HandleFunc("GET /openapi/v2/text-to-3d/ref-1", func(w http.ResponseWriter, r *http.Request) {
		n := refineGets.Add(1)
		st := "IN_PROGRESS"
		if n >= 2 {
			st = "SUCCEEDED"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "ref-1",
			"status": st,
			"mode":   "refine",
			"model_urls": map[string]any{
				"glb": srv.URL + "/m.glb",
				"fbx": srv.URL + "/m.fbx",
			},
			"texture_urls": []any{
				map[string]any{"base_color": srv.URL + "/bc.png"},
			},
		})
	})
	mux.HandleFunc("GET /m.glb", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("GLB")) })
	mux.HandleFunc("GET /m.fbx", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("FBX")) })
	mux.HandleFunc("GET /bc.png", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("PNG")) })

	dbPath := filepath.Join(t.TempDir(), "ez.db")
	outDir := filepath.Join(t.TempDir(), "out")
	var out bytes.Buffer
	app := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app, []string{
		"--api-key", "k", "--base-url", srv.URL, "--db", dbPath, "--json",
		"--poll-interval", "1ms", "--quiet",
		"ez", "textured-3d",
		"--prompt", "beefcake minotaur",
		"--name", "minotaur",
		"--profile", "game-hero",
		"-o", outDir,
	}); err != nil {
		t.Fatal(err)
	}
	var man map[string]any
	if err := json.Unmarshal(out.Bytes(), &man); err != nil {
		t.Fatalf("%v %s", err, out.String())
	}
	if man["preview_id"] != "prev-1" || man["refine_id"] != "ref-1" {
		t.Fatalf("ids: %s", out.String())
	}
	files, _ := man["files"].(map[string]any)
	if _, ok := files["glb"]; !ok {
		t.Fatalf("expected glb file: %s", out.String())
	}
	// game-hero is glb only by default
	if _, ok := files["fbx"]; ok {
		t.Fatalf("game-hero should not download fbx by default: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadMultiFormatAndTextures(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("GET /openapi/v1/image-to-3d/t1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "t1",
			"status": "SUCCEEDED",
			"model_urls": map[string]any{
				"glb": srv.URL + "/a.glb",
				"fbx": srv.URL + "/a.fbx",
			},
			"texture_urls": []any{
				map[string]any{"base_color": srv.URL + "/bc.png", "normal": srv.URL + "/n.png"},
			},
		})
	})
	mux.HandleFunc("GET /a.glb", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("G")) })
	mux.HandleFunc("GET /a.fbx", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("F")) })
	mux.HandleFunc("GET /bc.png", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("B")) })
	mux.HandleFunc("GET /n.png", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("N")) })

	dbPath := filepath.Join(t.TempDir(), "d.db")
	outDir := filepath.Join(t.TempDir(), "dl")
	var out bytes.Buffer
	app := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
	if err := cli.ExecuteWithApp(app, []string{
		"--api-key", "k", "--base-url", srv.URL, "--db", dbPath, "--json",
		"image-to-3d", "download", "t1",
		"--format", "glb,fbx", "--textures",
		"--name", "hero", "-o", outDir,
	}); err != nil {
		t.Fatal(err)
	}
	var dl map[string]any
	if err := json.Unmarshal(out.Bytes(), &dl); err != nil {
		t.Fatal(err)
	}
	files, _ := dl["files"].(map[string]any)
	for _, want := range []string{"glb", "fbx", "base_color", "normal"} {
		if _, ok := files[want]; !ok {
			t.Fatalf("missing %s in %v", want, files)
		}
	}
}

func TestMissingAuthClearError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "noauth.db")
	_ = os.Unsetenv("MESHY_API_KEY")
	app := &cli.App{Print: &cli.Printer{Out: io.Discard, Err: io.Discard}}
	err := cli.ExecuteWithApp(app, []string{
		"--db", dbPath,
		"balance", "get",
	})
	if err == nil {
		t.Fatal("expected missing API key error")
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateFlagsCoverage(t *testing.T) {
	var gotBodies []map[string]any
	mux := http.NewServeMux()
	capture := func(path string) {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
			}
			gotBodies = append(gotBodies, body)
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "task-x"})
		})
	}
	capture("/openapi/v1/retexture")
	capture("/openapi/v1/animations")
	capture("/openapi/v1/print/multi-color")
	capture("/openapi/v1/image-to-3d")
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dbPath := filepath.Join(t.TempDir(), "flags.db")
	run := func(args ...string) {
		var out bytes.Buffer
		app := &cli.App{Print: &cli.Printer{JSON: true, Out: &out, Err: io.Discard}}
		full := append([]string{"--api-key", "k", "--base-url", srv.URL, "--db", dbPath, "--json"}, args...)
		if err := cli.ExecuteWithApp(app, full); err != nil {
			t.Fatalf("args %v: %v", args, err)
		}
	}

	run("retexture", "create",
		"--input-task-id", "src-1",
		"--text-style-prompt", "gold armor",
		"--enable-original-uv",
		"--enable-pbr",
		"--texture-resolution", "4k",
		"--target-formats", "glb,fbx",
	)
	run("animation", "create",
		"--rig-task-id", "rig-1",
		"--action-id", "92",
		"--post-process-operation", "change_fps",
		"--post-process-fps", "24",
	)
	run("print", "multi-color", "create",
		"--input-task-id", "src-2",
		"--max-colors", "8",
	)
	run("image-to-3d", "create",
		"--image-url", "https://example.com/a.png",
		"--should-texture",
		"--should-remesh",
		"--enable-pbr",
		"--pose-mode", "a-pose",
		"--decimation-mode", "2",
		"--alpha-thumbnail",
		"--multi-view-thumbnails",
		"--auto-size",
		"--origin-at", "center",
		"--target-formats", "glb",
	)

	if len(gotBodies) != 4 {
		t.Fatalf("expected 4 creates, got %d", len(gotBodies))
	}

	// retexture
	r := gotBodies[0]
	if r["text_style_prompt"] != "gold armor" {
		t.Fatalf("retexture text_style_prompt: %#v", r)
	}
	if r["enable_original_uv"] != true {
		t.Fatalf("enable_original_uv: %#v", r)
	}
	formats, _ := r["target_formats"].([]any)
	if len(formats) != 2 {
		t.Fatalf("target_formats: %#v", r["target_formats"])
	}

	// animation post_process
	a := gotBodies[1]
	pp, _ := a["post_process"].(map[string]any)
	if pp == nil || pp["operation_type"] != "change_fps" {
		t.Fatalf("post_process: %#v", a)
	}
	if int(pp["fps"].(float64)) != 24 {
		t.Fatalf("fps: %#v", pp)
	}
	if int(a["action_id"].(float64)) != 92 {
		t.Fatalf("action_id: %#v", a)
	}

	// multi-color
	p := gotBodies[2]
	if int(p["max_colors"].(float64)) != 8 {
		t.Fatalf("max_colors: %#v", p)
	}

	// image-to-3d
	i := gotBodies[3]
	for _, key := range []string{"should_texture", "should_remesh", "enable_pbr", "alpha_thumbnail", "multi_view_thumbnails", "auto_size"} {
		if i[key] != true {
			t.Fatalf("%s not true in %#v", key, i)
		}
	}
	if i["pose_mode"] != "a-pose" || i["origin_at"] != "center" {
		t.Fatalf("pose/origin: %#v", i)
	}
	if int(i["decimation_mode"].(float64)) != 2 {
		t.Fatalf("decimation_mode: %#v", i)
	}
}
