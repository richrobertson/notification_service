package delivery

import (
	"bytes"
	"fmt"
	"text/template"
)

func RenderTemplate(body string, variables map[string]any) (string, error) {
	tmpl, err := template.New("body").Option("missingkey=error").Parse(body)
	if err != nil {
		return "", fmt.Errorf("parse template body: %w", err)
	}
	if variables == nil {
		variables = map[string]any{}
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, variables); err != nil {
		return "", fmt.Errorf("render template body: %w", err)
	}
	return buf.String(), nil
}
