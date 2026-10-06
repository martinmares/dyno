package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoggingMiddlewareLevels(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	for _, tt := range []struct {
		name   string
		status int
		level  string
	}{
		{"implicit success", 0, "DEBUG"},
		{"polling success", http.StatusOK, "DEBUG"},
		{"redirect", http.StatusFound, "DEBUG"},
		{"not modified", http.StatusNotModified, "DEBUG"},
		{"client error", http.StatusNotFound, "WARN"},
		{"server error", http.StatusInternalServerError, "ERROR"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, minimum := range []slog.Level{slog.LevelInfo, slog.LevelDebug} {
				var output bytes.Buffer
				slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: minimum})))
				handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if tt.status != 0 {
						w.WriteHeader(tt.status)
					}
				}))
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/browse/_status", nil))
				status := tt.status
				if status == 0 {
					status = http.StatusOK
				}
				if response.Code != status {
					t.Fatalf("response status = %d, want %d", response.Code, status)
				}
				if minimum == slog.LevelInfo && tt.level == "DEBUG" {
					if output.Len() != 0 {
						t.Fatalf("successful request logged at INFO: %s", output.String())
					}
					continue
				}
				var entry struct {
					Level    string `json:"level"`
					Message  string `json:"msg"`
					Method   string `json:"method"`
					Path     string `json:"path"`
					Status   int    `json:"status"`
					Duration int64  `json:"duration"`
				}
				if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
					t.Fatalf("decode log: %v", err)
				}
				if entry.Level != tt.level || entry.Status != status || entry.Message != "request completed" || entry.Method != http.MethodGet || entry.Path != "/browse/_status" || entry.Duration < 0 {
					t.Fatalf("unexpected request log: %+v", entry)
				}
			}
		})
	}
}
