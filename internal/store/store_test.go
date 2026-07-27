package store_test

import (
	"path/filepath"
	"testing"

	"github.com/crertel/meshy-cli/internal/store"
)

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meshy.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.SetSetting("api_key", "msy_secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting("base_url", "https://api.example"); err != nil {
		t.Fatal(err)
	}

	// Reopen to prove durability
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	v, ok, err := s2.GetSetting("api_key")
	if err != nil || !ok || v != "msy_secret" {
		t.Fatalf("api_key = %q ok=%v err=%v", v, ok, err)
	}
	v, ok, err = s2.GetSetting("base_url")
	if err != nil || !ok || v != "https://api.example" {
		t.Fatalf("base_url = %q ok=%v err=%v", v, ok, err)
	}
	all, err := s2.ListSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("list = %#v", all)
	}
}

func TestSessionAndTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meshy.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sess, err := s.CreateSession("batch-1", "my batch")
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID != "batch-1" {
		t.Fatalf("id = %s", sess.ID)
	}

	if err := s.UpsertTask(store.TrackedTask{
		ID:         "task-a",
		Resource:   "image-to-3d",
		SessionID:  "batch-1",
		Status:     "PENDING",
		RemotePath: "/openapi/v1/image-to-3d/task-a",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTags("task-a", []string{"hero", "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertTask(store.TrackedTask{
		ID:        "task-b",
		Resource:  "remesh",
		SessionID: "batch-1",
		Status:    "SUCCEEDED",
	}); err != nil {
		t.Fatal(err)
	}

	// reopen
	_ = s.Close()
	s2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	bySession, err := s2.ListTasks(store.TaskFilter{SessionID: "batch-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(bySession) != 2 {
		t.Fatalf("session tasks = %#v", bySession)
	}

	byTag, err := s2.ListTasks(store.TaskFilter{Tag: "hero"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byTag) != 1 || byTag[0].ID != "task-a" {
		t.Fatalf("tag filter = %#v", byTag)
	}
	if len(byTag[0].Tags) != 2 {
		t.Fatalf("tags = %#v", byTag[0].Tags)
	}

	if err := s2.CloseSession("batch-1"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s2.GetSession("batch-1")
	if err != nil || !ok || got.ClosedAt == nil {
		t.Fatalf("closed session = %#v ok=%v err=%v", got, ok, err)
	}
}
