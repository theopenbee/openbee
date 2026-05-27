package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"text/template"
)

func TestConfigTemplatePreservesLinearMaxMediaSize(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(`
server:
  port: 8080
bee:
  platforms:
    linear:
      - name: default
        enabled: true
        api_key: "lin_test"
        label_name: "openbee"
        poll_interval: 15s
        projects: ["Backend"]
        states: ["Todo"]
        max_media_size: 10485760
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	vals := loadExistingConfig(f.Name())
	if vals == nil {
		t.Fatal("loadExistingConfig returned nil")
	}

	if got := vals.LinearMaxMediaSize; got != "10485760" {
		t.Fatalf("LinearMaxMediaSize = %q, want 10485760", got)
	}

	vals.LinearProjectsYAML = renderInlineYAMLList(vals.LinearProjects)
	vals.LinearStatesYAML = renderInlineYAMLList(vals.LinearStates)

	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, vals); err != nil {
		t.Fatalf("render template: %v", err)
	}
	if !strings.Contains(out.String(), "max_media_size: 10485760") {
		t.Fatalf("rendered config missing linear max_media_size:\n%s", out.String())
	}
}
