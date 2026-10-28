package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crertel/meshy-cli/internal/api"
)

func TestCreateGetListDelete(t *testing.T) {
	var gets atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /openapi/v1/image-to-3d", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth header = %q", got)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["image_url"] != "https://example.com/a.png" {
			t.Errorf("body = %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "task-123"})
	})
	mux.HandleFunc("GET /openapi/v1/image-to-3d/task-123", func(w http.ResponseWriter, r *http.Request) {
		n := gets.Add(1)
		status := "IN_PROGRESS"
		progress := 50.0
		if n >= 3 {
			status = "SUCCEEDED"
			progress = 100
		}
		resp := map[string]any{
			"id":       "task-123",
			"status":   status,
			"progress": progress,
		}
		if status == "SUCCEEDED" {
			resp["model_urls"] = map[string]string{"glb": "https://cdn.example/m.glb"}
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("GET /openapi/v1/image-to-3d", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "task-123", "status": "SUCCEEDED"},
		})
	})
	mux.HandleFunc("DELETE /openapi/v1/image-to-3d/task-123", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := api.NewClient(srv.URL, "test-key", srv.Client())
	ctx := context.Background()

	id, err := c.Create(ctx, "/openapi/v1/image-to-3d", map[string]any{
		"image_url": "https://example.com/a.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "task-123" {
		t.Fatalf("id = %q", id)
	}

	task, err := c.Wait(ctx, "/openapi/v1/image-to-3d/task-123", api.WaitOptions{
		Interval: 1 * time.Millisecond,
		Timeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Status() != "SUCCEEDED" {
		t.Fatalf("status = %q", task.Status())
	}
	if task.ModelURLs()["glb"] == "" {
		t.Fatal("missing glb url")
	}
	if gets.Load() < 3 {
		t.Fatalf("expected multiple polls, got %d", gets.Load())
	}

	list, err := c.List(ctx, "/openapi/v1/image-to-3d", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID() != "task-123" {
		t.Fatalf("list = %#v", list)
	}

	if err := c.Delete(ctx, "/openapi/v1/image-to-3d/task-123"); err != nil {
		t.Fatal(err)
	}
}

func TestWaitFailed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi/v2/text-to-3d/fail-1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "fail-1",
			"status": "FAILED",
			"task_error": map[string]string{
				"message": "bad prompt",
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := api.NewClient(srv.URL, "k", srv.Client())
	task, err := c.Wait(context.Background(), "/openapi/v2/text-to-3d/fail-1", api.WaitOptions{
		Interval: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Status() != "FAILED" {
		t.Fatalf("status = %s", task.Status())
	}
	if task.TaskErrorMessage() != "bad prompt" {
		t.Fatalf("error msg = %q", task.TaskErrorMessage())
	}
}

func TestMissingAPIKey(t *testing.T) {
	c := api.NewClient("http://example", "", nil)
	_, err := c.Create(context.Background(), "/openapi/v1/image-to-3d", map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got == "" || !contains(got, "API key") {
		t.Fatalf("error = %q", got)
	}
}

func TestGetBalance(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi/v1/balance", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"balance": 42})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := api.NewClient(srv.URL, "k", srv.Client())
	bal, err := c.GetBalance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bal["balance"] == nil {
		t.Fatalf("balance = %#v", bal)
	}
}

func TestWaitStream(t *testing.T) {
	var n atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi/v1/image-to-3d/s1/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"id\":\"s1\",\"status\":\"IN_PROGRESS\",\"progress\":10}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		n.Add(1)
		fmt.Fprintf(w, "data: {\"id\":\"s1\",\"status\":\"SUCCEEDED\",\"progress\":100,\"model_urls\":{\"glb\":\"https://x/a.glb\"}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := api.NewClient(srv.URL, "k", srv.Client())
	task, err := c.WaitStream(context.Background(), "/openapi/v1/image-to-3d/s1/stream", api.WaitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if task.Status() != "SUCCEEDED" || task.ID() != "s1" {
		t.Fatalf("%#v", task)
	}
}

func TestResourceMapCoversFamilies(t *testing.T) {
	groups := map[string]bool{}
	for _, r := range api.AllResources {
		groups[r.Group] = true
		if r.Path == "" || r.Name == "" {
			t.Fatalf("invalid resource %#v", r)
		}
	}
	for _, g := range []string{"generation", "postprocess", "image", "print", "creative-lab"} {
		if !groups[g] {
			t.Errorf("missing group %s", g)
		}
	}
	if _, ok := api.ResourceByName("text-to-3d"); !ok {
		t.Fatal("text-to-3d missing")
	}
	if _, ok := api.ResourceByName("image-to-3d"); !ok {
		t.Fatal("image-to-3d missing")
	}
	for _, name := range []string{"print-split", "text-to-motion", "cl-brick-figure-build", "cl-keycap-build", "cl-fidget-pixel-prototype", "cl-fidget-collapsible"} {
		r, ok := api.ResourceByName(name)
		if !ok {
			t.Fatalf("missing resource %s", name)
		}
		if !r.HasList || !r.HasDelete || r.Path == "" {
			t.Fatalf("%s incomplete: %#v", name, r)
		}
	}
	for _, name := range []string{"rigging", "animation"} {
		r, ok := api.ResourceByName(name)
		if !ok || !r.HasList {
			t.Fatalf("%s should list", name)
		}
	}
	wantModels := map[string]bool{
		"nano-banana-2-lite":     false,
		"gpt-image-2-5-flare":    false,
		"gpt-image-2-5-sunburst": false,
	}
	for _, m := range api.KnownImageModels {
		if _, ok := wantModels[m.ID]; ok {
			wantModels[m.ID] = true
		}
	}
	for id, seen := range wantModels {
		if !seen {
			t.Errorf("image model catalog missing %s", id)
		}
	}
}

func TestCreateMetadataHeader(t *testing.T) {
	var gotMeta []string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /openapi/v1/image-to-3d", func(w http.ResponseWriter, r *http.Request) {
		gotMeta = append(gotMeta, r.Header.Get("X-Meshy-Metadata"))
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "t1"})
	})
	mux.HandleFunc("POST /openapi/creative-lab/keychain/v1/prototype", func(w http.ResponseWriter, r *http.Request) {
		gotMeta = append(gotMeta, "cl:"+r.Header.Get("X-Meshy-Metadata"))
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "t2"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := api.NewClient(srv.URL, "k", srv.Client())
	c.Metadata = "user=alex@example.com,team=fab"
	if _, err := c.Create(context.Background(), "/openapi/v1/image-to-3d", map[string]any{"image_url": "https://x/a.png"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Create(context.Background(), "/openapi/creative-lab/keychain/v1/prototype", map[string]any{"image_url": "https://x/a.png"}); err != nil {
		t.Fatal(err)
	}
	if len(gotMeta) != 2 || gotMeta[0] != "user=alex@example.com,team=fab" || gotMeta[1] != "cl:" {
		t.Fatalf("metadata headers: %#v", gotMeta)
	}
}

func TestListSortAndUsageQuery(t *testing.T) {
	var got []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /openapi/v1/image-to-3d", func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.RawQuery)
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	})
	mux.HandleFunc("GET /openapi/v1/usage/tasks", func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.RawQuery)
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	})
	mux.HandleFunc("GET /openapi/v1/animations/library", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Errorf("library missing auth")
		}
		got = append(got, r.URL.RawQuery)
		_, _ = w.Write([]byte(`[{"action_id":92}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := api.NewClient(srv.URL, "k", srv.Client())
	if _, err := c.ListSorted(context.Background(), "/openapi/v1/image-to-3d", 2, 5, "-created_at"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListUsage(context.Background(), api.UsageFilter{
		PageNum: 1, PageSize: 50, StartTime: "2026-08-01T00:00:00Z", Endpoints: "image-to-3d", Status: "SUCCEEDED",
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := c.AnimationLibrary(context.Background(), api.AnimationLibraryQuery{Category: "Fighting", Search: "walk"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "92") {
		t.Fatalf("library body %s", raw)
	}
	joined := strings.Join(got, " | ")
	for _, want := range []string{"sort_by=-created_at", "page_num=2", "start_time=", "endpoints=image-to-3d", "status=SUCCEEDED", "category=Fighting", "search=walk"} {
		if !strings.Contains(joined, want) {
			t.Errorf("query %q missing from %s", want, joined)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
