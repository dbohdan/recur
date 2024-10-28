package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"text/template"

	"github.com/mitchellh/go-wordwrap"
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

	funcMap := template.FuncMap{
		"wrap": func(width uint, s string) (string, error) {
			return wordwrap.WrapString(s, width), nil
		},
	}

	tmpl, err := template.New("template").Funcs(funcMap).Parse(string(templateData))
	if err != nil {
		log.Fatalf("Failed to parse template: %v", err)
	}

	data := struct {
		Help string
	}{
		Help: cmdOutput.String(),
	}

	if err := tmpl.Execute(os.Stdout, data); err != nil {
		log.Fatalf("Failed to execute template: %v", err)
	}
}
