package delivery

import (
	"bytes"
	"fmt"
	"text/template"
)

// RenderTemplate applies `{{variable}}` substitutions to a template body.
//
// The renderer is intentionally small and explicit. It is suitable for message
// bodies and operator-visible examples, not for generalized templating logic.
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
