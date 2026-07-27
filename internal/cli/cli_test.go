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
	for _, want := range []string{"create", "get", "list", "delete", "wait", "download"} {
		if !strings.Contains(sub, want) {
			t.Errorf("image-to-3d help missing %q\n%s", want, sub)
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
