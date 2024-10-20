package main

import (
	"bytes"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
)

func main() {
	templateData, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read template: %v", err)
	}

	cmd := exec.Command("./recur", "--help")
	var cmdOutput bytes.Buffer
	cmd.Stdout = &cmdOutput
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to run command: %v", err)
	}

	tmpl, err := template.New("template").Parse(string(templateData))
	if err != nil {
		log.Fatalf("Failed to parse template: %v", err)
	}

	data := struct {
		Help template.HTML
	}{
		Help: template.HTML(cmdOutput.String()),
	}

	if err := tmpl.Execute(os.Stdout, data); err != nil {
		log.Fatalf("Failed to execute template: %v", err)
	}
}
