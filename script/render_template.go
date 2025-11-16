//go:build ignore

package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os/exec"
	"regexp"
	"strings"

	"github.com/mitchellh/go-wordwrap"
)

func main() {
	readmeFile := "README.md"
	content, err := ioutil.ReadFile(readmeFile)
	if err != nil {
		log.Fatalf(`Failed to read "README.md": %v`, err)
	}

	cmd := exec.Command("./recur", "--help")
	var cmdOutput bytes.Buffer
	cmd.Stdout = &cmdOutput
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to run command: %v", err)
	}

	helpText := strings.TrimSpace(cmdOutput.String())
	wrappedHelp := wordwrap.WrapString(helpText, 80)

	re := regexp.MustCompile(`(?s)<!-- BEGIN USAGE -->.*<!-- END USAGE -->`)
	newUsageBlock := "<!-- BEGIN USAGE -->\n```none\n" + wrappedHelp + "\n```\n<!-- END USAGE -->"
	updatedContent := re.ReplaceAllLiteralString(string(content), newUsageBlock)

	if err := ioutil.WriteFile(readmeFile, []byte(updatedContent), 0644); err != nil {
		log.Fatalf(`Failed to write "README.md": %v`, err)
	}
}
