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
	ID         string    `json:"id"`
	Event      string    `json:"event,omitempty"`
	DocumentID string    `json:"document_id"`
	Kind       string    `json:"kind,omitempty"`
	PagePath   string    `json:"page_path"`
	ParentID   string    `json:"parent_id,omitempty"`
	Anchor     string    `json:"anchor,omitempty"`
	Quote      string    `json:"quote,omitempty"`
	Author     string    `json:"author"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

// Store persists and reads comments.
type Store interface {
	List(documentID, legacyPagePath string) ([]Comment, error)
	Add(comment Comment) (Comment, error)
	Update(comment Comment) (Comment, error)
	Delete(documentID, pagePath, id string) error
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

func (s *JSONLStore) List(documentID, legacyPagePath string) ([]Comment, error) {
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

	items := make(map[string]Comment)
	var order []string
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
		if c.DocumentID == documentID || (c.DocumentID == "" && c.PagePath == legacyPagePath) {
			if c.Event == "delete" {
				delete(items, c.ID)
				continue
			}
			if _, exists := items[c.ID]; !exists {
				order = append(order, c.ID)
			}
			items[c.ID] = c
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	var out []Comment
	for _, id := range order {
		if item, ok := items[id]; ok {
			out = append(out, item)
		}
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
	comment.DocumentID = truncate(strings.TrimSpace(comment.DocumentID), 240)
	comment.Kind = strings.ToLower(strings.TrimSpace(comment.Kind))
	if comment.Kind == "" {
		comment.Kind = "comment"
	}
	comment.ParentID = strings.TrimSpace(comment.ParentID)
	comment.Anchor = strings.TrimSpace(comment.Anchor)
	comment.Quote = truncate(strings.TrimSpace(comment.Quote), 500)
	comment.Author = truncate(strings.TrimSpace(comment.Author), 120)
	comment.Body = truncate(strings.TrimSpace(comment.Body), 4000)
	if comment.PagePath == "" || comment.DocumentID == "" {
		return Comment{}, ErrValidation
	}
	if comment.Kind != "comment" && comment.Kind != "highlight" {
		return Comment{}, ErrValidation
	}
	if comment.Kind == "comment" && comment.Body == "" {
		return Comment{}, ErrValidation
	}
	if comment.Kind == "highlight" && comment.Quote == "" {
		return Comment{}, ErrValidation
	}
	if comment.Kind == "comment" && comment.Author == "" {
		comment.Author = "Anonymous"
	}
	if comment.ID == "" {
		comment.ID = newID()
	}
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = time.Now().UTC()
	}

	return comment, s.append(comment)
}

func (s *JSONLStore) Update(comment Comment) (Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	comment.ID = strings.TrimSpace(comment.ID)
	comment.DocumentID = strings.TrimSpace(comment.DocumentID)
	comment.PagePath = normalizePagePath(comment.PagePath)
	comment.Body = truncate(strings.TrimSpace(comment.Body), 4000)
	comment.Quote = truncate(strings.TrimSpace(comment.Quote), 500)
	comment.Anchor = strings.TrimSpace(comment.Anchor)
	if comment.ID == "" || comment.DocumentID == "" || (comment.Kind == "comment" && comment.Body == "") || (comment.Kind == "highlight" && comment.Quote == "") {
		return Comment{}, ErrValidation
	}
	comment.Event = "update"
	comment.UpdatedAt = time.Now().UTC()
	return comment, s.append(comment)
}

func (s *JSONLStore) Delete(documentID, pagePath, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(documentID) == "" || strings.TrimSpace(id) == "" {
		return ErrValidation
	}
	return s.append(Comment{ID: id, Event: "delete", DocumentID: documentID, PagePath: normalizePagePath(pagePath), UpdatedAt: time.Now().UTC()})
}

func (s *JSONLStore) append(comment Comment) error {
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	enc, err := json.Marshal(comment)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(enc, '\n')); err != nil {
		return err
	}
	return nil
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
