package delivery

import "testing"

func TestRenderTemplate(t *testing.T) {
	t.Parallel()

	got, err := RenderTemplate(`{"name":"{{.name}}"}`, map[string]any{"name": "Ada"})
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}
	if got != `{"name":"Ada"}` {
		t.Fatalf("RenderTemplate() = %q", got)
	}
}

func TestRenderTemplateMissingVariable(t *testing.T) {
	t.Parallel()

	_, err := RenderTemplate(`hello {{.name}}`, map[string]any{})
	if err == nil {
		t.Fatal("RenderTemplate() error = nil, want missing variable error")
	}
}
