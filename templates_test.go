package main

import "testing"

func TestTemplatesLoad(t *testing.T) {
	appConfig = Config{
		Server: ServerConfig{Debug: false},
	}

	engine := createTemplateEngine()
	if engine == nil {
		t.Fatal("engine de templates es nil")
	}

	if err := engine.Load(); err != nil {
		t.Fatalf("error cargando/parsing templates: %v", err)
	}
}

