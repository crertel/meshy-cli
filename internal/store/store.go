// Package store provides SQLite persistence for settings, sessions, tags, and tracked tasks.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store is a local SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) a store at path.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("empty store path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DBPath is the default path under the user config directory.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "meshy", "meshy.db"), nil
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  closed_at INTEGER
);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  resource TEXT NOT NULL,
  session_id TEXT,
  status TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  remote_path TEXT,
  FOREIGN KEY(session_id) REFERENCES sessions(id)
);

CREATE TABLE IF NOT EXISTS tags (
  task_id TEXT NOT NULL,
  tag TEXT NOT NULL,
  PRIMARY KEY(task_id, tag),
  FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tasks_session ON tasks(session_id);
CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);

CREATE TABLE IF NOT EXISTS profiles (
  name TEXT PRIMARY KEY,
  description TEXT NOT NULL DEFAULT '',
  data_json TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
`
	_, err := s.db.Exec(schema)
	return err
}

// --- Settings ---

// SetSetting writes a key/value pair.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}

// GetSetting returns a setting value and whether it exists.
func (s *Store) GetSetting(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// ListSettings returns all settings.
func (s *Store) ListSettings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// DeleteSetting removes a setting.
func (s *Store) DeleteSetting(key string) error {
	_, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, key)
	return err
}

// --- Sessions ---

// Session is a local work session for batch job tracking.
type Session struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	ClosedAt  *int64 `json:"closed_at,omitempty"`
}

// CreateSession creates a named session with the given id.
func (s *Store) CreateSession(id, name string) (Session, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO sessions(id, name, created_at, closed_at) VALUES(?, ?, ?, NULL)`,
		id, name, now,
	)
	if err != nil {
		return Session{}, err
	}
	return Session{ID: id, Name: name, CreatedAt: now}, nil
}

// GetSession returns a session by id.
func (s *Store) GetSession(id string) (Session, bool, error) {
	var sess Session
	var closed sql.NullInt64
	err := s.db.QueryRow(
		`SELECT id, name, created_at, closed_at FROM sessions WHERE id = ?`, id,
	).Scan(&sess.ID, &sess.Name, &sess.CreatedAt, &closed)
	if err == sql.ErrNoRows {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	if closed.Valid {
		v := closed.Int64
		sess.ClosedAt = &v
	}
	return sess, true, nil
}

// ListSessions returns sessions, optionally only open ones.
func (s *Store) ListSessions(openOnly bool) ([]Session, error) {
	q := `SELECT id, name, created_at, closed_at FROM sessions`
	if openOnly {
		q += ` WHERE closed_at IS NULL`
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		var closed sql.NullInt64
		if err := rows.Scan(&sess.ID, &sess.Name, &sess.CreatedAt, &closed); err != nil {
			return nil, err
		}
		if closed.Valid {
			v := closed.Int64
			sess.ClosedAt = &v
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// CloseSession marks a session closed.
func (s *Store) CloseSession(id string) error {
	res, err := s.db.Exec(
		`UPDATE sessions SET closed_at = ? WHERE id = ? AND closed_at IS NULL`,
		time.Now().UnixMilli(), id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session %q not found or already closed", id)
	}
	return nil
}

// --- Tasks ---

// TrackedTask is a locally tracked remote Meshy task.
type TrackedTask struct {
	ID         string   `json:"id"`
	Resource   string   `json:"resource"`
	SessionID  string   `json:"session_id,omitempty"`
	Status     string   `json:"status,omitempty"`
	CreatedAt  int64    `json:"created_at"`
	UpdatedAt  int64    `json:"updated_at"`
	RemotePath string   `json:"remote_path,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

// UpsertTask inserts or updates a tracked task.
func (s *Store) UpsertTask(t TrackedTask) error {
	now := time.Now().UnixMilli()
	if t.CreatedAt == 0 {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	_, err := s.db.Exec(
		`INSERT INTO tasks(id, resource, session_id, status, created_at, updated_at, remote_path)
		 VALUES(?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   resource=excluded.resource,
		   session_id=COALESCE(excluded.session_id, tasks.session_id),
		   status=COALESCE(excluded.status, tasks.status),
		   updated_at=excluded.updated_at,
		   remote_path=COALESCE(excluded.remote_path, tasks.remote_path)`,
		t.ID, t.Resource, nullStr(t.SessionID), nullStr(t.Status), t.CreatedAt, t.UpdatedAt, nullStr(t.RemotePath),
	)
	return err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// AddTags attaches tags to a task.
func (s *Store) AddTags(taskID string, tags []string) error {
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, err := s.db.Exec(
			`INSERT OR IGNORE INTO tags(task_id, tag) VALUES(?, ?)`,
			taskID, tag,
		); err != nil {
			return err
		}
	}
	return nil
}

// TaskFilter filters local tasks.
type TaskFilter struct {
	SessionID string
	Tag       string
	Resource  string
	Status    string
}

// ListTasks returns tracked tasks matching the filter.
func (s *Store) ListTasks(f TaskFilter) ([]TrackedTask, error) {
	q := `SELECT t.id, t.resource, COALESCE(t.session_id,''), COALESCE(t.status,''),
	             t.created_at, t.updated_at, COALESCE(t.remote_path,'')
	      FROM tasks t`
	var args []any
	var where []string
	if f.Tag != "" {
		q += ` INNER JOIN tags tg ON tg.task_id = t.id`
		where = append(where, `tg.tag = ?`)
		args = append(args, f.Tag)
	}
	if f.SessionID != "" {
		where = append(where, `t.session_id = ?`)
		args = append(args, f.SessionID)
	}
	if f.Resource != "" {
		where = append(where, `t.resource = ?`)
		args = append(args, f.Resource)
	}
	if f.Status != "" {
		where = append(where, `t.status = ?`)
		args = append(args, f.Status)
	}
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += ` ORDER BY t.created_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrackedTask
	seen := map[string]int{}
	for rows.Next() {
		var t TrackedTask
		if err := rows.Scan(&t.ID, &t.Resource, &t.SessionID, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.RemotePath); err != nil {
			return nil, err
		}
		if i, ok := seen[t.ID]; ok {
			_ = i
			continue
		}
		seen[t.ID] = len(out)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		tags, err := s.tagsFor(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Tags = tags
	}
	return out, nil
}

func (s *Store) tagsFor(taskID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT tag FROM tags WHERE task_id = ? ORDER BY tag`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// GetTask returns a tracked task.
func (s *Store) GetTask(id string) (TrackedTask, bool, error) {
	var t TrackedTask
	err := s.db.QueryRow(
		`SELECT id, resource, COALESCE(session_id,''), COALESCE(status,''), created_at, updated_at, COALESCE(remote_path,'')
		 FROM tasks WHERE id = ?`, id,
	).Scan(&t.ID, &t.Resource, &t.SessionID, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.RemotePath)
	if err == sql.ErrNoRows {
		return TrackedTask{}, false, nil
	}
	if err != nil {
		return TrackedTask{}, false, err
	}
	tags, err := s.tagsFor(id)
	if err != nil {
		return TrackedTask{}, false, err
	}
	t.Tags = tags
	return t, true, nil
}

// UpdateTaskStatus updates status and timestamp.
func (s *Store) UpdateTaskStatus(id, status string) error {
	_, err := s.db.Exec(
		`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UnixMilli(), id,
	)
	return err
}

// --- User profiles (JSON blobs) ---

// StoredProfile is a user-defined profile row.
type StoredProfile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	DataJSON    string `json:"data_json"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// UpsertProfile inserts or updates a user profile by name.
func (s *Store) UpsertProfile(name, description, dataJSON string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(
		`INSERT INTO profiles(name, description, data_json, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   description=excluded.description,
		   data_json=excluded.data_json,
		   updated_at=excluded.updated_at`,
		name, description, dataJSON, now, now,
	)
	return err
}

// GetProfile returns a user profile by name.
func (s *Store) GetProfile(name string) (StoredProfile, bool, error) {
	var p StoredProfile
	err := s.db.QueryRow(
		`SELECT name, description, data_json, created_at, updated_at FROM profiles WHERE name = ?`,
		name,
	).Scan(&p.Name, &p.Description, &p.DataJSON, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return StoredProfile{}, false, nil
	}
	if err != nil {
		return StoredProfile{}, false, err
	}
	return p, true, nil
}

// ListProfiles returns all user profiles ordered by name.
func (s *Store) ListProfiles() ([]StoredProfile, error) {
	rows, err := s.db.Query(
		`SELECT name, description, data_json, created_at, updated_at FROM profiles ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredProfile
	for rows.Next() {
		var p StoredProfile
		if err := rows.Scan(&p.Name, &p.Description, &p.DataJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteProfile removes a user profile.
func (s *Store) DeleteProfile(name string) error {
	res, err := s.db.Exec(`DELETE FROM profiles WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("profile %q not found", name)
	}
	return nil
}
