package comments

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Comment is a user note attached to a rendered documentation page.
type Comment struct {
	ID        string    `json:"id"`
	PagePath  string    `json:"page_path"`
	ParentID  string    `json:"parent_id,omitempty"`
	Anchor    string    `json:"anchor,omitempty"`
	Quote     string    `json:"quote,omitempty"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Store persists and reads comments.
type Store interface {
	List(pagePath string) ([]Comment, error)
	Add(comment Comment) (Comment, error)
}

var ErrValidation = errors.New("invalid comment")

// JSONLStore stores comments as append-only JSON Lines.
type JSONLStore struct {
	mu   sync.Mutex
	path string
}

func NewJSONLStore(path string) (*JSONLStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("comments store path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &JSONLStore{path: path}, nil
}

func (s *JSONLStore) List(pagePath string) ([]Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var out []Comment
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var c Comment
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		if c.PagePath == pagePath {
			out = append(out, c)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *JSONLStore) Add(comment Comment) (Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	comment.PagePath = normalizePagePath(comment.PagePath)
	comment.ParentID = strings.TrimSpace(comment.ParentID)
	comment.Anchor = strings.TrimSpace(comment.Anchor)
	comment.Quote = truncate(strings.TrimSpace(comment.Quote), 500)
	comment.Author = truncate(strings.TrimSpace(comment.Author), 120)
	comment.Body = truncate(strings.TrimSpace(comment.Body), 4000)
	if comment.PagePath == "" {
		return Comment{}, ErrValidation
	}
	if comment.Body == "" {
		return Comment{}, ErrValidation
	}
	if comment.Author == "" {
		comment.Author = "Anonymous"
	}
	if comment.ID == "" {
		comment.ID = newID()
	}
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = time.Now().UTC()
	}

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return Comment{}, err
	}
	defer file.Close()

	enc, err := json.Marshal(comment)
	if err != nil {
		return Comment{}, err
	}
	if _, err := file.Write(append(enc, '\n')); err != nil {
		return Comment{}, err
	}
	return comment, nil
}

func normalizePagePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}
