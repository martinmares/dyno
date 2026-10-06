package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

//go:embed docs/examples.md
var examplesGuide string

func newExamplesCommand() *cobra.Command {
	var plain, markdown bool
	cmd := &cobra.Command{
		Use:     "examples [TOPIC]",
		Aliases: []string{"example"},
		Short:   "Show practical commands, configuration and Markdown examples",
		Long:    "Read the built-in Dyno cookbook without starting a server. Use a topic to show one section. Redirected output and NO_COLOR disable terminal colors. Use --markdown to export the source guide.",
		Args:    cobra.MaximumNArgs(1),
		Example: "  dyno examples\n  dyno examples browse\n  dyno examples markdown\n  dyno examples --markdown > dyno-examples.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			guide := examplesGuide
			if len(args) == 1 {
				var err error
				guide, err = examplesTopic(args[0])
				if err != nil {
					return err
				}
			}
			color := false
			if file, ok := cmd.OutOrStdout().(*os.File); ok {
				_, noColor := os.LookupEnv("NO_COLOR")
				color = !plain && !noColor && os.Getenv("TERM") != "dumb" && isatty.IsTerminal(file.Fd())
			}
			if markdown {
				_, err := io.WriteString(cmd.OutOrStdout(), guide)
				return err
			}
			return writeExamples(cmd.OutOrStdout(), guide, color)
		},
	}
	cmd.Flags().BoolVar(&plain, "plain", false, "Disable terminal colors")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "Export the original Markdown guide")
	return cmd
}

func examplesTopic(topic string) (string, error) {
	lines := strings.SplitAfter(examplesGuide, "\n")
	start, end := -1, len(lines)
	var topics []string
	fence := 0
	for i, line := range lines {
		if advanceExamplesFence(line, &fence) {
			continue
		}
		if fence != 0 || !strings.HasPrefix(line, "## ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "## "))
		topics = append(topics, name)
		if start >= 0 && end == len(lines) {
			end = i
		}
		if strings.EqualFold(name, topic) {
			start = i
		}
	}
	if start < 0 {
		return "", fmt.Errorf("unknown examples topic %q; choose: %s", topic, strings.Join(topics, ", "))
	}
	return strings.Join(lines[start:end], ""), nil
}

func writeExamples(w io.Writer, guide string, color bool) error {
	printLine := func(text, style string) error {
		if color && style != "" {
			text = style + text + "\x1b[0m"
		}
		_, err := fmt.Fprintln(w, text)
		return err
	}
	fence := 0
	blockStyle := ""
	pendingFile := ""
	for _, line := range strings.Split(strings.TrimSuffix(guide, "\n"), "\n") {
		text, style := line, "\x1b[2m"
		switch {
		case fence != 0:
			if advanceExamplesFence(line, &fence) {
				continue
			}
			text, style = "      "+line, blockStyle
		case advanceExamplesFence(line, &fence):
			format := strings.TrimSpace(line[fence:])
			blockStyle = "\x1b[1;34m"
			label := "FILE (" + strings.ToUpper(format) + ")"
			if format == "sh" || format == "bash" {
				blockStyle, label = "\x1b[1;32m", "SHELL"
			} else if pendingFile != "" {
				label = "FILE: " + pendingFile
			}
			pendingFile = ""
			text, style = "    "+label, blockStyle
		case strings.HasPrefix(line, "FILE: "):
			pendingFile = strings.TrimPrefix(line, "FILE: ")
			continue
		case strings.HasPrefix(line, "### "):
			text, style = "  > "+strings.TrimPrefix(line, "### "), "\x1b[1;33m"
		case strings.HasPrefix(line, "## "):
			text, style = "## "+strings.ToUpper(strings.TrimPrefix(line, "## ")), "\x1b[1;36m"
		case strings.HasPrefix(line, "# "):
			text, style = strings.ToUpper(strings.TrimPrefix(line, "# ")), "\x1b[1;36m"
		case line != "":
			text = "    " + strings.ReplaceAll(line, "`", "")
		}
		if err := printLine(text, style); err != nil {
			return err
		}
	}
	return nil
}

// Four-backtick examples can contain literal three-backtick Markdown fences.
func advanceExamplesFence(line string, fence *int) bool {
	count := len(line) - len(strings.TrimLeft(line, "`"))
	if count < 3 {
		return false
	}
	if *fence == 0 {
		*fence = count
		return true
	}
	if count >= *fence && strings.TrimSpace(line[count:]) == "" {
		*fence = 0
		return true
	}
	return false
}
