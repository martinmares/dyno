package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/mares/dyno/internal/mcpengine"
)

const ProtocolVersion = "2025-03-26"

type Server struct {
	engine       *mcpengine.Engine
	name         string
	version      string
	authToken    string
	allowOrigins map[string]struct{}
}

type Config struct {
	Name         string
	Version      string
	Engine       *mcpengine.Engine
	AuthToken    string
	AllowOrigins []string
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type tool struct {
	Name         string         `json:"name"`
	Title        string         `json:"title,omitempty"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	Annotations  map[string]any `json:"annotations,omitempty"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
}

func New(cfg Config) *Server {
	allowOrigins := make(map[string]struct{}, len(cfg.AllowOrigins))
	for _, origin := range cfg.AllowOrigins {
		normalized := strings.TrimSpace(origin)
		if normalized == "" {
			continue
		}
		allowOrigins[normalized] = struct{}{}
	}
	return &Server{
		engine:       cfg.Engine,
		name:         cfg.Name,
		version:      cfg.Version,
		authToken:    cfg.AuthToken,
		allowOrigins: allowOrigins,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("MCP-Protocol-Version", ProtocolVersion)
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.validateHTTPRequest(r); err != nil {
		status := http.StatusForbidden
		if strings.Contains(err.Error(), "authorization") {
			status = http.StatusUnauthorized
			w.Header().Set("WWW-Authenticate", `Bearer realm="dyno-mcp"`)
		} else if strings.Contains(err.Error(), "MCP-Protocol-Version") {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}

	defer r.Body.Close()
	responses, notificationsOnly, err := s.handlePayload(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errorResponse(nil, -32700, err.Error(), nil))
		return
	}
	if notificationsOnly {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if len(responses) == 1 {
		_ = json.NewEncoder(w).Encode(responses[0])
		return
	}
	_ = json.NewEncoder(w).Encode(responses)
}

func (s *Server) ServeStdio() error {
	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		responses, _, err := s.handlePayload(strings.NewReader(line))
		if err != nil {
			resp := errorResponse(nil, -32700, err.Error(), nil)
			if err := writeJSONLine(writer, resp); err != nil {
				return err
			}
			continue
		}
		for _, resp := range responses {
			if err := writeJSONLine(writer, resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func writeJSONLine(w *bufio.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return w.Flush()
}

func (s *Server) validateHTTPRequest(r *http.Request) error {
	if header := r.Header.Get("MCP-Protocol-Version"); header != "" && header != ProtocolVersion {
		return fmt.Errorf("unsupported MCP-Protocol-Version %q", header)
	}
	origin := r.Header.Get("Origin")
	if origin != "" {
		if _, ok := s.allowOrigins[origin]; !ok {
			return fmt.Errorf("origin not allowed")
		}
	}
	if s.authToken == "" {
		return nil
	}
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return fmt.Errorf("authorization required")
	}
	if strings.TrimPrefix(auth, prefix) != s.authToken {
		return fmt.Errorf("authorization failed")
	}
	return nil
}

func (s *Server) handlePayload(r io.Reader) ([]rpcResponse, bool, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, false, err
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, false, fmt.Errorf("empty request body")
	}
	if body[0] == '[' {
		var messages []json.RawMessage
		if err := json.Unmarshal(body, &messages); err != nil {
			return nil, false, err
		}
		if len(messages) == 0 {
			return nil, false, fmt.Errorf("empty batch is invalid")
		}
		responses := make([]rpcResponse, 0, len(messages))
		for _, item := range messages {
			resp, ok := s.handleMessage(item)
			if ok {
				responses = append(responses, resp)
			}
		}
		return responses, len(responses) == 0, nil
	}

	resp, ok := s.handleMessage(body)
	if !ok {
		return nil, true, nil
	}
	return []rpcResponse{resp}, false, nil
}

func (s *Server) handleMessage(raw json.RawMessage) (rpcResponse, bool) {
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errorResponse(nil, -32600, "invalid request", err.Error()), true
	}
	if req.JSONRPC != "2.0" {
		return errorResponse(req.idValue(), -32600, "invalid request", "jsonrpc must be 2.0"), true
	}

	result, err := s.dispatch(req)
	if len(req.ID) == 0 {
		if err != nil {
			slog.Warn("mcp notification failed", "method", req.Method, "err", err)
		}
		return rpcResponse{}, false
	}
	if err != nil {
		return errorResponse(req.idValue(), -32000, err.Error(), nil), true
	}
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      req.idValue(),
		Result:  result,
	}, true
}

func (r rpcRequest) idValue() interface{} {
	if len(r.ID) == 0 {
		return nil
	}
	var value interface{}
	if err := json.Unmarshal(r.ID, &value); err != nil {
		return string(r.ID)
	}
	return value
}

func errorResponse(id interface{}, code int, message string, data interface{}) rpcResponse {
	return rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

func (s *Server) dispatch(req rpcRequest) (interface{}, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
				"resources": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    s.name,
				"version": s.version,
			},
			"instructions": "Use dyno tools to search and read read-only documentation content. Prefer returning provided URLs when citing pages.",
		}, nil
	case "notifications/initialized":
		return nil, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.tools()}, nil
	case "tools/call":
		return s.callTool(req.Params)
	case "resources/list":
		return s.listResources(req.Params)
	case "resources/read":
		return s.readResource(req.Params)
	default:
		return nil, fmt.Errorf("method not found: %s", req.Method)
	}
}

func (s *Server) tools() []tool {
	return []tool{
		{
			Name:        "list_books",
			Title:       "List Dyno Books",
			Description: "List the documentation books exposed by dyno-mcp. In single-site mode this returns one implicit book.",
			InputSchema: objectSchema(nil, nil),
			Annotations: readOnlyAnnotations(),
			OutputSchema: objectSchema(map[string]any{
				"books": arraySchema(objectSchema(map[string]any{
					"slug":        strSchema("Stable book identifier"),
					"title":       strSchema("Display title"),
					"base_path":   strSchema("Book base path on the dyno site"),
					"public_url":  strSchema("Public dyno URL for the book root"),
					"description": strSchema("Book description from dyno.yaml"),
				}, []string{"slug", "title", "base_path", "public_url"}), "Available books"),
				"mode": strSchema("single or library"),
			}, []string{"books", "mode"}),
		},
		{
			Name:        "search_docs",
			Title:       "Search Dyno Docs",
			Description: "Full-text search across dyno documentation. Returns result titles, snippets, and URLs suitable for clickable references.",
			InputSchema: objectSchema(map[string]any{
				"query":     strSchema("Search query"),
				"book_slug": optionalStrSchema("Optional book slug to constrain the search"),
				"limit":     optionalIntSchema("Maximum number of results (default 10, max 50)"),
			}, []string{"query"}),
			Annotations: readOnlyAnnotations(),
			OutputSchema: objectSchema(map[string]any{
				"results": arraySchema(objectSchema(map[string]any{
					"book_slug":  strSchema("Book slug"),
					"book_title": strSchema("Book title"),
					"title":      strSchema("Matched page title"),
					"path":       strSchema("Book-relative page path"),
					"url":        strSchema("Public dyno URL"),
					"snippet":    strSchema("Matching snippet"),
					"score":      numberSchema("Ranking score"),
				}, []string{"book_slug", "book_title", "title", "path", "url", "snippet", "score"}), "Search hits"),
			}, []string{"results"}),
		},
		{
			Name:        "get_page",
			Title:       "Read Dyno Page",
			Description: "Read one dyno documentation page and return markdown, plain text, HTML, headings, metadata, and public URL.",
			InputSchema: objectSchema(map[string]any{
				"path":      strSchema("Book-relative path such as /guides/api-proxy/"),
				"book_slug": optionalStrSchema("Required in library mode, omitted in single-site mode"),
			}, []string{"path"}),
			Annotations: readOnlyAnnotations(),
		},
		{
			Name:        "get_navigation",
			Title:       "Get Dyno Navigation",
			Description: "Return the navigation tree as a flat ordered list for one dyno book.",
			InputSchema: objectSchema(map[string]any{
				"book_slug": optionalStrSchema("Required in library mode, omitted in single-site mode"),
			}, nil),
			Annotations: readOnlyAnnotations(),
		},
		{
			Name:        "list_pages",
			Title:       "List Dyno Pages",
			Description: "List page paths and public URLs for one dyno book, optionally filtered by prefix.",
			InputSchema: objectSchema(map[string]any{
				"book_slug": optionalStrSchema("Required in library mode, omitted in single-site mode"),
				"prefix":    optionalStrSchema("Optional relative path prefix filter such as /guides/"),
			}, nil),
			Annotations: readOnlyAnnotations(),
		},
		{
			Name:        "get_page_section",
			Title:       "Read Dyno Page Section",
			Description: "Read one heading section from a dyno page and return markdown, plain text, HTML, and the page URL.",
			InputSchema: objectSchema(map[string]any{
				"path":      strSchema("Book-relative path such as /guides/api-proxy/"),
				"heading":   strSchema("Exact heading text to extract"),
				"book_slug": optionalStrSchema("Required in library mode, omitted in single-site mode"),
			}, []string{"path", "heading"}),
			Annotations: readOnlyAnnotations(),
		},
	}
}

func (s *Server) callTool(raw json.RawMessage) (interface{}, error) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}

	switch params.Name {
	case "list_books":
		return s.callListBooks()
	case "search_docs":
		return s.callSearchDocs(params.Arguments)
	case "get_page":
		return s.callGetPage(params.Arguments)
	case "get_navigation":
		return s.callGetNavigation(params.Arguments)
	case "list_pages":
		return s.callListPages(params.Arguments)
	case "get_page_section":
		return s.callGetPageSection(params.Arguments)
	default:
		return nil, fmt.Errorf("unknown tool: %s", params.Name)
	}
}

func (s *Server) callListBooks() (map[string]any, error) {
	books := make([]map[string]any, 0, len(s.engine.ListBooks()))
	for _, book := range s.engine.ListBooks() {
		books = append(books, map[string]any{
			"slug":        book.Slug,
			"title":       book.Title,
			"base_path":   book.BasePath,
			"public_url":  publicBookURL(s.engine, book),
			"description": book.Cfg.Description,
		})
	}
	payload := map[string]any{
		"mode":  string(s.engine.Mode()),
		"books": books,
	}
	return toolResult(payload, fmt.Sprintf("Loaded %d dyno book(s).", len(books))), nil
}

func (s *Server) callSearchDocs(args map[string]interface{}) (map[string]any, error) {
	query, err := requiredStringArg(args, "query")
	if err != nil {
		return nil, err
	}
	bookSlug, err := optionalStringArg(args, "book_slug")
	if err != nil {
		return nil, err
	}
	limit, err := optionalIntArg(args, "limit")
	if err != nil {
		return nil, err
	}

	results, err := s.engine.Search(query, bookSlug, limit)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"results": results,
	}
	return toolResult(payload, fmt.Sprintf("Found %d dyno result(s) for %q.", len(results), query)), nil
}

func (s *Server) callGetPage(args map[string]interface{}) (map[string]any, error) {
	path, err := requiredStringArg(args, "path")
	if err != nil {
		return nil, err
	}
	bookSlug, err := optionalStringArg(args, "book_slug")
	if err != nil {
		return nil, err
	}
	page, err := s.engine.GetPage(bookSlug, path)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"page": page,
	}
	return toolResult(payload, fmt.Sprintf("Loaded dyno page %s (%s).", page.Title, page.URL)), nil
}

func (s *Server) callGetNavigation(args map[string]interface{}) (map[string]any, error) {
	bookSlug, err := optionalStringArg(args, "book_slug")
	if err != nil {
		return nil, err
	}
	items, err := s.engine.GetNavigation(bookSlug)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"items": items,
	}
	return toolResult(payload, fmt.Sprintf("Loaded %d navigation item(s).", len(items))), nil
}

func (s *Server) callListPages(args map[string]interface{}) (map[string]any, error) {
	bookSlug, err := optionalStringArg(args, "book_slug")
	if err != nil {
		return nil, err
	}
	prefix, err := optionalStringArg(args, "prefix")
	if err != nil {
		return nil, err
	}
	items, err := s.engine.ListPages(bookSlug, prefix)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"items": items,
	}
	return toolResult(payload, fmt.Sprintf("Loaded %d page item(s).", len(items))), nil
}

func (s *Server) callGetPageSection(args map[string]interface{}) (map[string]any, error) {
	path, err := requiredStringArg(args, "path")
	if err != nil {
		return nil, err
	}
	heading, err := requiredStringArg(args, "heading")
	if err != nil {
		return nil, err
	}
	bookSlug, err := optionalStringArg(args, "book_slug")
	if err != nil {
		return nil, err
	}
	section, err := s.engine.GetPageSection(bookSlug, path, heading)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"section": section,
	}
	return toolResult(payload, fmt.Sprintf("Loaded section %q from %s.", section.Heading, section.PageURL)), nil
}

func (s *Server) listResources(raw json.RawMessage) (interface{}, error) {
	var params struct {
		Cursor string `json:"cursor"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("invalid resources/list params: %w", err)
		}
	}
	resources, nextCursor, err := s.engine.ListResources(params.Cursor, 100)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"resources": resources,
	}
	if nextCursor != "" {
		result["nextCursor"] = nextCursor
	}
	return result, nil
}

func (s *Server) readResource(raw json.RawMessage) (interface{}, error) {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("invalid resources/read params: %w", err)
	}
	if strings.TrimSpace(params.URI) == "" {
		return nil, fmt.Errorf("uri is required")
	}
	contents, err := s.engine.ReadResource(strings.TrimSpace(params.URI))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"contents": contents,
	}, nil
}

func toolResult(payload map[string]any, summary string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": summary,
			},
		},
		"structuredContent": payload,
	}
}

func publicBookURL(engine *mcpengine.Engine, book *mcpengine.Book) string {
	page, err := engine.ListPages(book.Slug, "/")
	if err == nil && len(page) > 0 {
		rootURL := page[0].URL
		rootPath := page[0].Path
		if rootPath != "/" && strings.HasSuffix(rootURL, rootPath) {
			return strings.TrimSuffix(rootURL, rootPath) + "/"
		}
	}
	return book.BasePath
}

func readOnlyAnnotations() map[string]any {
	return map[string]any{
		"readOnlyHint":   true,
		"destructiveHint": false,
		"idempotentHint": true,
		"openWorldHint":  false,
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object"}
	if len(properties) > 0 {
		schema["properties"] = properties
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func arraySchema(items any, description string) map[string]any {
	schema := map[string]any{
		"type":  "array",
		"items": items,
	}
	if description != "" {
		schema["description"] = description
	}
	return schema
}

func strSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func optionalStrSchema(description string) map[string]any {
	return strSchema(description)
}

func optionalIntSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func numberSchema(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func requiredStringArg(args map[string]interface{}, name string) (string, error) {
	value, err := optionalStringArg(args, name)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalStringArg(args map[string]interface{}, name string) (string, error) {
	value, ok := args[name]
	if !ok || value == nil {
		return "", nil
	}
	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return strings.TrimSpace(s), nil
}

func optionalIntArg(args map[string]interface{}, name string) (int, error) {
	value, ok := args[name]
	if !ok || value == nil {
		return 0, nil
	}
	switch v := value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("%s must be an integer", name)
	}
}
