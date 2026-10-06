package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestExamplesCommand(t *testing.T) {
	for _, topic := range []string{"", "browse", "serve", "edit", "library", "git", "comments", "config", "markdown", "mcp", "BROWSE"} {
		t.Run(topic, func(t *testing.T) {
			cmd := newExamplesCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			args := []string{"--markdown"}
			if topic != "" {
				args = append(args, topic)
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "\x1b[") {
				t.Fatal("plain output contains ANSI escapes")
			}
			if topic == "" {
				if out.String() != examplesGuide {
					t.Fatal("full guide differs from embedded Markdown")
				}
			} else if !strings.HasPrefix(out.String(), "## "+strings.ToLower(topic)+"\n") || strings.Count(out.String(), "\n## ") != 0 {
				t.Fatalf("unexpected topic boundaries: %s", out.String())
			}
		})
	}
}

func TestExamplesInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"browse", "serve"}} {
		cmd := newExamplesCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted arguments %v", args)
		}
	}
}

func TestExamplesRendering(t *testing.T) {
	var out bytes.Buffer
	guide := "## markdown\n\n````markdown\n```sql\nSELECT 1;\n```\n````\n## mcp\n"
	if err := writeExamples(&out, guide, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\x1b[1;36m## MCP") || !strings.Contains(out.String(), "\x1b[1;34m      SELECT 1;") || strings.Contains(out.String(), "````markdown") || !strings.Contains(out.String(), "      ```sql") {
		t.Fatalf("unexpected colored output: %q", out.String())
	}
	for _, color := range []bool{true, false} {
		if err := writeExamples(failingExamplesWriter{}, guide, color); !errors.Is(err, errExamplesWrite) {
			t.Fatalf("write error not propagated: %v", err)
		}
	}
}

func TestExamplesCookbookLayout(t *testing.T) {
	cmd := newExamplesCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"browse", "--plain"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## BROWSE", "  > Open a single document", "    SHELL\n      dyno browse README.md"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing cookbook element %q", want)
		}
	}
	if strings.Contains(out.String(), "```") || strings.Contains(out.String(), "\x1b[") {
		t.Fatal("plain cookbook contains fences or ANSI escapes")
	}
	out.Reset()
	if err := writeExamples(&out, "### Site\nFILE: site/dyno.yaml\n\n```yaml\ntitle: Docs\n```\n", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "    FILE: site/dyno.yaml\n      title: Docs") {
		t.Fatalf("missing named file block: %s", out.String())
	}
}

var errExamplesWrite = errors.New("cannot write examples")

type failingExamplesWriter struct{}

func (failingExamplesWriter) Write([]byte) (int, error) { return 0, errExamplesWrite }
