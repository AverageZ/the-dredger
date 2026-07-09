package config

import "testing"

func TestLoad_DefaultLLMConfig(t *testing.T) {
	clearLLMEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.LLMService != LLMServiceOllama {
		t.Fatalf("LLMService = %q, want %q", cfg.LLMService, LLMServiceOllama)
	}
	if cfg.OllamaURL != "http://localhost:11434" {
		t.Errorf("OllamaURL = %q", cfg.OllamaURL)
	}
	if cfg.OllamaModel != "gemma4:e4b" {
		t.Errorf("OllamaModel = %q", cfg.OllamaModel)
	}
	if cfg.LMStudioURL != "http://127.0.0.1:1234" {
		t.Errorf("LMStudioURL = %q", cfg.LMStudioURL)
	}
	if cfg.LMStudioModel != "google/gemma-4-26b-a4b" {
		t.Errorf("LMStudioModel = %q", cfg.LMStudioModel)
	}
}

func TestLoad_LLMEnvOverrides(t *testing.T) {
	t.Setenv("DREDGER_LLM_SERVICE", LLMServiceLMStudio)
	t.Setenv("DREDGER_OLLAMA_URL", "http://ollama.test")
	t.Setenv("DREDGER_OLLAMA_MODEL", "ollama-model")
	t.Setenv("DREDGER_LMSTUDIO_URL", "http://lmstudio.test")
	t.Setenv("DREDGER_LMSTUDIO_MODEL", "lmstudio-model")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.LLMService != LLMServiceLMStudio {
		t.Errorf("LLMService = %q", cfg.LLMService)
	}
	if cfg.OllamaURL != "http://ollama.test" {
		t.Errorf("OllamaURL = %q", cfg.OllamaURL)
	}
	if cfg.OllamaModel != "ollama-model" {
		t.Errorf("OllamaModel = %q", cfg.OllamaModel)
	}
	if cfg.LMStudioURL != "http://lmstudio.test" {
		t.Errorf("LMStudioURL = %q", cfg.LMStudioURL)
	}
	if cfg.LMStudioModel != "lmstudio-model" {
		t.Errorf("LMStudioModel = %q", cfg.LMStudioModel)
	}
}

func TestLoad_InvalidLLMService(t *testing.T) {
	t.Setenv("DREDGER_LLM_SERVICE", "bogus")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLMService != "bogus" {
		t.Fatalf("LLMService = %q, want bogus", cfg.LLMService)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateLLMService(t *testing.T) {
	for _, service := range []string{LLMServiceOllama, LLMServiceLMStudio} {
		if err := (Config{LLMService: service}).Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", service, err)
		}
	}

	if err := (Config{LLMService: "other"}).Validate(); err == nil {
		t.Fatal("expected error for invalid service")
	}
}

func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"DREDGER_LLM_SERVICE",
		"DREDGER_OLLAMA_URL",
		"DREDGER_OLLAMA_MODEL",
		"DREDGER_LMSTUDIO_URL",
		"DREDGER_LMSTUDIO_MODEL",
	} {
		t.Setenv(key, "")
	}
}
