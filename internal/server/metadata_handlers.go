package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mares/dyno/internal/config"
	"github.com/mares/dyno/internal/frontmatter"
	"github.com/mares/dyno/internal/metadata"
	"gopkg.in/yaml.v3"
)

type MetadataFieldData struct {
	Name        string
	Label       string
	Type        string
	InputType   string
	Value       string
	Checked     bool
	Options     []MetadataOption
	ReadOnly    bool
	Required    bool
	Description string
}

type MetadataOption struct {
	Value    string
	Selected bool
}

type FrontmatterEntryData struct {
	Key   string
	Value string
}

type frontmatterEntryPayload struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type metadataApplyResponse struct {
	OK        bool   `json:"ok"`
	Source    string `json:"source,omitempty"`
	Error     string `json:"error,omitempty"`
	Code      string `json:"code,omitempty"`
	Revision  string `json:"revision,omitempty"`
	GitStatus string `json:"git_status,omitempty"`
}

func buildMetadataFields(cfg config.FrontmatterConfig, source []byte) ([]MetadataFieldData, error) {
	configured := cfg.OrderedFields()
	if len(configured) == 0 {
		return nil, nil
	}
	metadata, err := frontmatter.Read(source)
	if err != nil {
		return nil, err
	}
	fields := make([]MetadataFieldData, 0, len(configured))
	for _, field := range configured {
		value := metadata[field.Name]
		text, checked := metadataFieldValue(value, field.Type)
		label := field.Label
		if label == "" {
			label = field.Name
		}
		data := MetadataFieldData{
			Name:        field.Name,
			Label:       label,
			Type:        field.Type,
			InputType:   metadataInputType(field.Type),
			Value:       text,
			Checked:     checked,
			ReadOnly:    field.ReadOnly,
			Required:    field.Required,
			Description: field.Description,
		}
		for _, option := range field.Options {
			data.Options = append(data.Options, MetadataOption{Value: option, Selected: option == text})
		}
		fields = append(fields, data)
	}
	return fields, nil
}

func buildFrontmatterEntries(source []byte) ([]FrontmatterEntryData, error) {
	entries, err := frontmatter.ReadEntries(source)
	if err != nil {
		return nil, err
	}
	out := make([]FrontmatterEntryData, 0, len(entries))
	for _, entry := range entries {
		out = append(out, FrontmatterEntryData{Key: entry.Key, Value: entry.Value})
	}
	return out, nil
}

func metadataFieldValue(value any, fieldType string) (string, bool) {
	if value == nil {
		return "", false
	}
	if fieldType == "boolean" {
		checked, _ := strconv.ParseBool(fmt.Sprint(value))
		return "", checked
	}
	if fieldType == "tags" {
		switch typed := value.(type) {
		case []any:
			values := make([]string, 0, len(typed))
			for _, item := range typed {
				values = append(values, fmt.Sprint(item))
			}
			return strings.Join(values, ", "), false
		case []string:
			return strings.Join(typed, ", "), false
		}
	}
	if timestamp, ok := value.(time.Time); ok {
		switch fieldType {
		case "date":
			return timestamp.Format("2006-01-02"), false
		case "datetime":
			return timestamp.Format("2006-01-02T15:04"), false
		default:
			return timestamp.Format(time.RFC3339), false
		}
	}
	switch value.(type) {
	case map[string]any, []any:
		encoded, err := yaml.Marshal(value)
		if err == nil {
			return strings.TrimSpace(string(encoded)), false
		}
	}
	return fmt.Sprint(value), false
}

func metadataInputType(fieldType string) string {
	switch fieldType {
	case "number", "date":
		return fieldType
	case "datetime":
		return "datetime-local"
	default:
		return "text"
	}
}

func (s *Server) editMetadataHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	if err := r.ParseForm(); err != nil {
		writeMetadataResponse(w, http.StatusBadRequest, metadataApplyResponse{Error: "invalid form", Code: "invalid_request"})
		return
	}
	source := r.FormValue("source")
	var submitted map[string]any
	if err := json.Unmarshal([]byte(r.FormValue("values")), &submitted); err != nil {
		writeMetadataResponse(w, http.StatusBadRequest, metadataApplyResponse{Error: "invalid metadata values", Code: "invalid_values"})
		return
	}

	updates := make(map[string]any, len(submitted))
	for name, value := range submitted {
		field, ok := s.siteCfg.Frontmatter.Fields[name]
		if !ok || field.ReadOnly {
			writeMetadataResponse(w, http.StatusBadRequest, metadataApplyResponse{Error: "metadata field is not editable: " + name, Code: "field_not_editable"})
			return
		}
		coerced, err := coerceMetadataValue(field, value)
		if err != nil {
			writeMetadataResponse(w, http.StatusBadRequest, metadataApplyResponse{Error: err.Error(), Code: "invalid_field"})
			return
		}
		updates[name] = coerced
	}

	updated, err := frontmatter.Apply([]byte(source), updates)
	if err != nil {
		writeMetadataResponse(w, http.StatusBadRequest, metadataApplyResponse{Error: err.Error(), Code: "invalid_frontmatter"})
		return
	}
	writeMetadataResponse(w, http.StatusOK, metadataApplyResponse{OK: true, Source: string(updated)})
}

func (s *Server) editFrontmatterHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	if err := r.ParseForm(); err != nil {
		writeMetadataResponse(w, http.StatusBadRequest, metadataApplyResponse{Error: "invalid form", Code: "invalid_request"})
		return
	}
	pagePath := strings.TrimSpace(r.FormValue("page_path"))
	expectedRevision := strings.TrimSpace(r.FormValue("revision"))
	var submitted []frontmatterEntryPayload
	if err := json.Unmarshal([]byte(r.FormValue("entries")), &submitted); err != nil {
		writeMetadataResponse(w, http.StatusBadRequest, metadataApplyResponse{Error: "invalid frontmatter entries", Code: "invalid_entries"})
		return
	}
	if pagePath == "" || expectedRevision == "" {
		writeMetadataResponse(w, http.StatusBadRequest, metadataApplyResponse{Error: "page_path and revision are required", Code: "invalid_request"})
		return
	}

	fsPath, err := s.editableFilePath(pagePath)
	if err != nil {
		writeMetadataResponse(w, http.StatusNotFound, metadataApplyResponse{Error: "document not found", Code: "not_found"})
		return
	}
	nav := s.getNav()

	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.readDocument(fsPath)
	if err != nil {
		writeMetadataResponse(w, http.StatusInternalServerError, metadataApplyResponse{Error: "failed to read document", Code: "read_failed"})
		return
	}
	currentRevision := contentRevision(current)
	if currentRevision != expectedRevision {
		writeMetadataResponse(w, http.StatusConflict, metadataApplyResponse{Error: "document changed on disk; reload before saving", Code: "conflict", Revision: currentRevision})
		return
	}

	entries := make([]frontmatter.Entry, 0, len(submitted))
	for _, item := range submitted {
		entries = append(entries, frontmatter.Entry{
			Key:   strings.TrimSpace(item.Key),
			Value: item.Value,
		})
	}
	updated, err := frontmatter.ApplyEntries(current, entries)
	if err != nil {
		writeMetadataResponse(w, http.StatusBadRequest, metadataApplyResponse{Error: err.Error(), Code: "invalid_frontmatter"})
		return
	}
	newRevision := contentRevision(updated)
	if newRevision == currentRevision {
		writeMetadataResponse(w, http.StatusOK, metadataApplyResponse{OK: true, Revision: currentRevision, Source: string(updated)})
		return
	}

	info, err := s.documentStat(fsPath)
	if err != nil {
		writeMetadataResponse(w, http.StatusInternalServerError, metadataApplyResponse{Error: "failed to inspect document", Code: "stat_failed"})
		return
	}
	if err := s.writeDocument(fsPath, updated, info.Mode().Perm()); err != nil {
		writeMetadataResponse(w, http.StatusInternalServerError, metadataApplyResponse{Error: "failed to write document", Code: "write_failed"})
		return
	}

	delete(s.pageCache, fsPath)
	if idx, err := metadata.Build(nav, s.siteCfg.Frontmatter); err == nil {
		s.metadata = idx
	}

	writeMetadataResponse(w, http.StatusOK, metadataApplyResponse{OK: true, Source: string(updated), Revision: newRevision, GitStatus: gitFileStatus(fsPath)})
}

func coerceMetadataValue(field config.FrontmatterFieldConfig, value any) (any, error) {
	name := field.Label
	if name == "" {
		name = "metadata field"
	}
	if value == nil {
		if field.Required {
			return nil, fmt.Errorf("%s is required", name)
		}
		return nil, nil
	}
	switch field.Type {
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("%s must be true or false", name)
		}
		return boolean, nil
	case "number":
		number, ok := value.(float64)
		if !ok {
			return nil, fmt.Errorf("%s must be a number", name)
		}
		return number, nil
	case "tags":
		items, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("%s must be a list", name)
		}
		values := make([]string, 0, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s must contain text values", name)
			}
			text = strings.TrimSpace(text)
			if text != "" {
				values = append(values, text)
			}
		}
		if field.Required && len(values) == 0 {
			return nil, fmt.Errorf("%s is required", name)
		}
		return values, nil
	default:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be text", name)
		}
		text = strings.TrimSpace(text)
		if field.Required && text == "" {
			return nil, fmt.Errorf("%s is required", name)
		}
		if text == "" {
			return text, nil
		}
		if field.Type == "date" {
			if _, err := time.Parse("2006-01-02", text); err != nil {
				return nil, fmt.Errorf("%s must be a valid date", name)
			}
		}
		if field.Type == "datetime" {
			if _, err := time.Parse("2006-01-02T15:04", text); err != nil {
				if _, rfcErr := time.Parse(time.RFC3339, text); rfcErr != nil {
					return nil, fmt.Errorf("%s must be a valid date and time", name)
				}
			}
		}
		if field.Type == "select" && !slices.Contains(field.Options, text) {
			return nil, fmt.Errorf("%s has an unsupported value", name)
		}
		return text, nil
	}
}

func writeMetadataResponse(w http.ResponseWriter, status int, response metadataApplyResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
