package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/frontmatter"
	"github.com/mares/dyno/internal/metadata"
	"github.com/mares/dyno/internal/navigation"
	"gopkg.in/yaml.v3"
)

type bulkFrontmatterColumnData struct {
	Name        string
	Label       string
	Type        string
	InputType   string
	Options     []MetadataOption
	Default     string
	ReadOnly    bool
	Required    bool
	Description string
}

type bulkFrontmatterRowData struct {
	Title    string
	FullPath string
	Revision string
	Values   []string
}

type bulkFrontmatterEditorData struct {
	Title   string
	Count   int
	Query   string
	SaveURL string
	Columns []bulkFrontmatterColumnData
	Rows    []bulkFrontmatterRowData
}

type bulkFrontmatterRowPayload struct {
	PagePath string            `json:"page_path"`
	Revision string            `json:"revision"`
	Values   map[string]string `json:"values"`
}

type bulkFrontmatterSaveResult struct {
	PagePath string `json:"page_path"`
	OK       bool   `json:"ok"`
	Revision string `json:"revision,omitempty"`
	Error    string `json:"error,omitempty"`
	Code     string `json:"code,omitempty"`
}

type bulkFrontmatterSaveResponse struct {
	OK   bool                        `json:"ok"`
	Rows []bulkFrontmatterSaveResult `json:"rows"`
}

func (s *Server) bulkFrontmatterHandler(w http.ResponseWriter, r *http.Request) {
	if !s.editMode {
		http.NotFound(w, r)
		return
	}
	view := s.metadataView(r)
	if !view.Active {
		http.Error(w, "metadata filter is required", http.StatusBadRequest)
		return
	}
	columns := bulkFrontmatterColumns(s.siteCfg.Frontmatter)
	if len(columns) == 0 {
		http.Error(w, "no bulk-editable frontmatter defaults configured", http.StatusBadRequest)
		return
	}
	rows, err := s.bulkFrontmatterRows(view.Allowed, columns)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	data := bulkFrontmatterEditorData{
		Title:   "Bulk edit frontmatter",
		Count:   len(rows),
		Query:   view.Query,
		SaveURL: s.editBasePath() + "/frontmatter/bulk/save" + view.Query,
		Columns: columns,
		Rows:    rows,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := s.getTemplate()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "bulk-frontmatter.html", data); err != nil {
		s.internalError(w, r, err)
	}
}

func (s *Server) bulkFrontmatterSaveHandler(w http.ResponseWriter, r *http.Request) {
	if !s.editMode {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var submitted []bulkFrontmatterRowPayload
	if err := json.Unmarshal([]byte(r.FormValue("rows")), &submitted); err != nil {
		http.Error(w, "invalid rows payload", http.StatusBadRequest)
		return
	}
	if len(submitted) == 0 {
		writeJSON(w, http.StatusBadRequest, bulkFrontmatterSaveResponse{OK: false})
		return
	}

	rows := make([]bulkFrontmatterSaveResult, 0, len(submitted))
	changed := false
	for _, row := range submitted {
		result := bulkFrontmatterSaveResult{PagePath: row.PagePath}
		if strings.TrimSpace(row.PagePath) == "" || strings.TrimSpace(row.Revision) == "" {
			result.Error = "page_path and revision are required"
			result.Code = "invalid_request"
			rows = append(rows, result)
			continue
		}
		fsPath, err := s.editableFilePath(row.PagePath)
		if err != nil {
			result.Error = "document not found"
			result.Code = "not_found"
			rows = append(rows, result)
			continue
		}

		s.mu.Lock()
		current, err := s.readDocument(fsPath)
		if err != nil {
			s.mu.Unlock()
			result.Error = "failed to read document"
			result.Code = "read_failed"
			rows = append(rows, result)
			continue
		}
		currentRevision := contentRevision(current)
		if currentRevision != row.Revision {
			s.mu.Unlock()
			result.Error = "document changed on disk; reload before saving"
			result.Code = "conflict"
			result.Revision = currentRevision
			rows = append(rows, result)
			continue
		}

		updated, err := applyBulkFrontmatterUpdates(current, row.Values)
		if err != nil {
			s.mu.Unlock()
			result.Error = err.Error()
			result.Code = "invalid_frontmatter"
			rows = append(rows, result)
			continue
		}
		newRevision := contentRevision(updated)
		if newRevision == currentRevision {
			s.mu.Unlock()
			result.OK = true
			result.Revision = currentRevision
			rows = append(rows, result)
			continue
		}
		info, err := s.documentStat(fsPath)
		if err != nil {
			s.mu.Unlock()
			result.Error = "failed to inspect document"
			result.Code = "stat_failed"
			rows = append(rows, result)
			continue
		}
		if err := s.writeDocument(fsPath, updated, info.Mode().Perm()); err != nil {
			s.mu.Unlock()
			result.Error = "failed to write document"
			result.Code = "write_failed"
			rows = append(rows, result)
			continue
		}
		delete(s.pageCache, fsPath)
		s.mu.Unlock()

		changed = true
		result.OK = true
		result.Revision = newRevision
		rows = append(rows, result)
	}

	if changed {
		if idx, err := metadata.Build(s.getNav(), s.siteCfg.Frontmatter); err == nil {
			s.mu.Lock()
			s.metadata = idx
			s.mu.Unlock()
		}
	}

	ok := true
	for _, row := range rows {
		if !row.OK {
			ok = false
			break
		}
	}
	writeJSON(w, http.StatusOK, bulkFrontmatterSaveResponse{OK: ok, Rows: rows})
}

func bulkFrontmatterColumns(cfg config.FrontmatterConfig) []bulkFrontmatterColumnData {
	fields := cfg.DefaultFields()
	columns := make([]bulkFrontmatterColumnData, 0, len(fields))
	for _, field := range fields {
		defaultValue := fmt.Sprint(cfg.Defaults[field.Name])
		label := field.Label
		if label == "" {
			label = field.Name
		}
		column := bulkFrontmatterColumnData{
			Name:        field.Name,
			Label:       label,
			Type:        field.Type,
			InputType:   metadataInputType(field.Type),
			Default:     defaultValue,
			ReadOnly:    field.ReadOnly,
			Required:    field.Required,
			Description: field.Description,
		}
		for _, option := range field.Options {
			column.Options = append(column.Options, MetadataOption{Value: option, Selected: false})
		}
		columns = append(columns, column)
	}
	return columns
}

func (s *Server) bulkFrontmatterRows(allowed map[string]bool, columns []bulkFrontmatterColumnData) ([]bulkFrontmatterRowData, error) {
	var rows []bulkFrontmatterRowData
	nav := s.getNav()
	walk := func(node *navigation.NavNode) {}
	walk = func(node *navigation.NavNode) {
		if node == nil {
			return
		}
		if node.FSPath != "" && (allowed == nil || allowed[node.FullPath]) {
			source, err := s.readDocument(node.FSPath)
			if err != nil {
				return
			}
			entries, err := frontmatter.ReadEntries(source)
			if err != nil {
				return
			}
			values := make([]string, 0, len(columns))
			entryMap := make(map[string]string, len(entries))
			for _, entry := range entries {
				entryMap[entry.Key] = entry.Value
			}
			for _, column := range columns {
				values = append(values, entryMap[column.Name])
			}
			revision := contentRevision(source)
			rows = append(rows, bulkFrontmatterRowData{
				Title:    node.Title,
				FullPath: node.FullPath,
				Revision: revision,
				Values:   values,
			})
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(nav)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].FullPath < rows[j].FullPath })
	return rows, nil
}

func applyBulkFrontmatterUpdates(source []byte, updates map[string]string) ([]byte, error) {
	if len(updates) == 0 {
		return append([]byte(nil), source...), nil
	}
	ordered := make([]string, 0, len(updates))
	for key := range updates {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)

	out := append([]byte(nil), source...)
	for _, name := range ordered {
		raw := strings.TrimSpace(updates[name])
		if raw == "" {
			continue
		}
		var parsed any
		if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, fmt.Errorf("%w: invalid value for %q: %v", frontmatter.ErrInvalid, name, err)
		}
		next, err := frontmatter.Apply(out, map[string]any{name: parsed})
		if err != nil {
			return nil, err
		}
		out = next
	}
	return out, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
