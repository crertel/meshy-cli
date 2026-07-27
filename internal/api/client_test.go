package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
