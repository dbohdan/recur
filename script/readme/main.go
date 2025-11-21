package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mitchellh/go-wordwrap"
)

const (
	exitError    = 1
	exitBadUsage = 2
	filePerms    = 0o644
	readmeFile   = "README.md"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s command [arg ...]\n", filepath.Base(os.Args[0]))
		os.Exit(exitBadUsage)
	}

	command := os.Args[1:]

	content, err := os.ReadFile(readmeFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read %q: %v\n", readmeFile, err)
		os.Exit(exitError)
	}

	cmd := exec.Command(command[0], command[1:]...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run command: %v\n", err)
		os.Exit(exitError)
	}

	helpText := strings.TrimSpace(string(output))
	wrappedHelp := wordwrap.WrapString(helpText, 80)

	re := regexp.MustCompile(`(?s)<!-- BEGIN USAGE -->.*<!-- END USAGE -->`)
	newUsageBlock := "<!-- BEGIN USAGE -->\n```none\n" + wrappedHelp + "\n```\n<!-- END USAGE -->"
	updatedContent := re.ReplaceAllLiteralString(string(content), newUsageBlock)

	if err := os.WriteFile(readmeFile, []byte(updatedContent), filePerms); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write %q: %v\n", readmeFile, err)
		os.Exit(exitError)
	}
}
