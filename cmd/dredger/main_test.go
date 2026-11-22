package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alexzajac/the-dredger/internal/config"
)

const (
	serviceLMStudioFlag = "--service=lmstudio"
	flagWebOutput       = "--out"
	invalidLLMService   = "bogus"
	testSite            = "site"
)

func TestApplyRootFlags_ServiceOverride(t *testing.T) {
	cfg := config.Config{LLMService: config.LLMServiceOllama}

	args, err := applyRootFlags(&cfg, []string{serviceLMStudioFlag})
	if err != nil {
		t.Fatalf("applyRootFlags: %v", err)
	}

	if cfg.LLMService != config.LLMServiceLMStudio {
		t.Fatalf("LLMService = %q, want %q", cfg.LLMService, config.LLMServiceLMStudio)
	}
	if len(args) != 0 {
		t.Fatalf("args = %v, want empty", args)
	}
}

func TestApplyRootFlags_ServiceOverridesInvalidConfig(t *testing.T) {
	cfg := config.Config{LLMService: invalidLLMService}

	args, err := applyRootFlags(&cfg, []string{serviceLMStudioFlag})
	if err != nil {
		t.Fatalf("applyRootFlags: %v", err)
	}

	if cfg.LLMService != config.LLMServiceLMStudio {
		t.Fatalf("LLMService = %q, want %q", cfg.LLMService, config.LLMServiceLMStudio)
	}
	if len(args) != 0 {
		t.Fatalf("args = %v, want empty", args)
	}
}

func TestApplyRootFlags_DefaultService(t *testing.T) {
	cfg := config.Config{LLMService: config.LLMServiceOllama}

	args, err := applyRootFlags(&cfg, nil)
	if err != nil {
		t.Fatalf("applyRootFlags: %v", err)
	}

	if cfg.LLMService != config.LLMServiceOllama {
		t.Fatalf("LLMService = %q, want %q", cfg.LLMService, config.LLMServiceOllama)
	}
	if len(args) != 0 {
		t.Fatalf("args = %v, want empty", args)
	}
}

func TestApplyRootFlags_InvalidService(t *testing.T) {
	cfg := config.Config{LLMService: config.LLMServiceOllama}

	if _, err := applyRootFlags(&cfg, []string{"--service=" + invalidLLMService}); err == nil {
		t.Fatal("expected invalid service error")
	}
}

func TestApplyRootFlags_SubcommandArgsUnchanged(t *testing.T) {
	cfg := config.Config{LLMService: config.LLMServiceOllama}

	args, err := applyRootFlags(&cfg, []string{commandWeb, flagWebOutput, testSite})
	if err != nil {
		t.Fatalf("applyRootFlags: %v", err)
	}

	want := []string{commandWeb, flagWebOutput, testSite}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func TestApplyRootFlags_ServiceBeforeSubcommand(t *testing.T) {
	cfg := config.Config{LLMService: config.LLMServiceOllama}

	args, err := applyRootFlags(&cfg, []string{serviceLMStudioFlag, commandWeb, flagWebOutput, testSite})
	if err != nil {
		t.Fatalf("applyRootFlags: %v", err)
	}

	if cfg.LLMService != config.LLMServiceLMStudio {
		t.Fatalf("LLMService = %q, want %q", cfg.LLMService, config.LLMServiceLMStudio)
	}
	want := []string{commandWeb, flagWebOutput, testSite}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func TestCLIProcess(t *testing.T) {
	if os.Getenv("DREDGER_TEST_PROCESS") != "1" {
		return
	}
	os.Args = append(os.Args[:1], os.Args[3:]...)
	main()
	os.Exit(0)
}

func TestCLIWithoutDatabase(t *testing.T) {
	const flagHelp = "--help"
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"root help", []string{flagHelp}, 0, rootUsage},
		{"short help", []string{"-h"}, 0, rootUsage},
		{"help command", []string{"help"}, 0, rootUsage},
		{"search help", []string{commandSearch, queryShader, flagHelp}, 0, searchUsage},
		{"show help", []string{commandShow, flagHelp}, 0, showUsage},
		{"tags help", []string{commandTags, flagHelp}, 0, tagsUsage},
		{"unknown command", []string{"find"}, 2, "Unknown command"},
		{"unknown root flag", []string{"--bogus"}, 2, "flag provided but not defined"},
		{"missing query", []string{commandSearch}, 2, searchUsage},
		{"extra show argument", []string{commandShow, "1", "2"}, 2, showUsage},
		{"extra tags argument", []string{commandTags, "oops"}, 2, tagsUsage},
		{"nonterminal UI", nil, 2, "needs a terminal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "missing.db")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			args := append([]string{"-test.run=^TestCLIProcess$", "--"}, tt.args...)
			cmd := exec.CommandContext(ctx, os.Args[0], args...)
			cmd.Env = append(os.Environ(), "DREDGER_TEST_PROCESS=1",
				"DREDGER_DB_PATH="+dbPath, "DREDGER_LOG_PATH="+dir,
				"DREDGER_LLM_SERVICE=ollama")
			out, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("CLI hung: %v", ctx.Err())
			}
			code := 0
			if err != nil {
				exit, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if code != tt.code || !strings.Contains(string(out), tt.want) {
				t.Fatalf("exit=%d, want %d; output=%s", code, tt.code, out)
			}
			if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
				t.Fatalf("help/usage created a database or stat failed: %v", err)
			}
		})
	}
}
