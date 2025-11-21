package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/mitchellh/go-wordwrap"
)

func main() {
	readmeFile := "README.md"

	content, err := os.ReadFile(readmeFile)
	if err != nil {
		log.Fatalf("Failed to read %q: %v", readmeFile, err)
	}

	var cmdOutput bytes.Buffer
	cmd := exec.Command("./recur", "--help")
	cmd.Stdout = &cmdOutput

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to run command: %v", err)
	}

	helpText := strings.TrimSpace(cmdOutput.String())
	wrappedHelp := wordwrap.WrapString(helpText, 80)

	re := regexp.MustCompile(`(?s)<!-- BEGIN USAGE -->.*<!-- END USAGE -->`)
	newUsageBlock := "<!-- BEGIN USAGE -->\n```none\n" + wrappedHelp + "\n```\n<!-- END USAGE -->"
	updatedContent := re.ReplaceAllLiteralString(string(content), newUsageBlock)

	if err := os.WriteFile(readmeFile, []byte(updatedContent), 0644); err != nil {
		log.Fatalf("Failed to write %q: %v", readmeFile, err)
	}
}
