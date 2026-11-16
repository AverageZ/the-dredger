package main

import (
	"errors"
	"flag"
	"io"
	"reflect"
	"testing"
)

const (
	flagSearchLimit = "--limit"
	flagJSON        = "--json"
	flagTag         = "--tag"
	queryProcedural = "procedural"
	queryGeneration = "generation"
	queryRendering  = "rendering"
	queryShader     = "shader"
	tagGraphics     = "graphics"
	tagWebGL        = "webgl"
	testBogus       = "bogus"
)

// newTestFlagSet mirrors the flag surface of `dredger search` closely enough to
// exercise parseInterspersed without running the command.
func newTestFlagSet() (*flag.FlagSet, *int, *bool, *stringList) {
	fs := flag.NewFlagSet(commandSearch, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 10, "")
	asJSON := fs.Bool("json", false, "")
	var tags stringList
	fs.Var(&tags, "tag", "")
	return fs, limit, asJSON, &tags
}

func TestParseInterspersed(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantWords []string
		wantLimit int
		wantJSON  bool
		wantTags  []string
	}{
		{
			name:      "flags before query",
			args:      []string{flagSearchLimit, "3", queryProcedural, queryGeneration},
			wantWords: []string{queryProcedural, queryGeneration},
			wantLimit: 3,
		},
		{
			// The case that made size-limit outrank Shader Lab: a plain
			// fs.Parse folds trailing flags into the query.
			name:      "flags after query",
			args:      []string{queryProcedural, queryGeneration, flagSearchLimit, "3"},
			wantWords: []string{queryProcedural, queryGeneration},
			wantLimit: 3,
		},
		{
			name:      "flags on both sides",
			args:      []string{flagJSON, queryShader, flagSearchLimit, "5"},
			wantWords: []string{queryShader},
			wantLimit: 5,
			wantJSON:  true,
		},
		{
			name:      "flag between query words",
			args:      []string{"wave", flagSearchLimit, "2", "collapse"},
			wantWords: []string{"wave", "collapse"},
			wantLimit: 2,
		},
		{
			name:      "equals form",
			args:      []string{queryShader, flagSearchLimit + "=7"},
			wantWords: []string{queryShader},
			wantLimit: 7,
		},
		{
			name:      "quoted phrase stays one word",
			args:      []string{"llm agents", flagJSON},
			wantWords: []string{"llm agents"},
			wantLimit: 10,
			wantJSON:  true,
		},
		{
			name:      "repeatable tag flag",
			args:      []string{queryRendering, flagTag, tagGraphics, flagTag, tagWebGL},
			wantWords: []string{queryRendering},
			wantLimit: 10,
			wantTags:  []string{tagGraphics, tagWebGL},
		},
		{
			name:      "comma separated tags",
			args:      []string{queryRendering, flagTag, tagGraphics + "," + tagWebGL},
			wantWords: []string{queryRendering},
			wantLimit: 10,
			wantTags:  []string{tagGraphics, tagWebGL},
		},
		{
			name:      "no args",
			args:      nil,
			wantWords: nil,
			wantLimit: 10,
		},
		{
			name:      "only flags",
			args:      []string{flagJSON},
			wantWords: nil,
			wantLimit: 10,
			wantJSON:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, limit, asJSON, tags := newTestFlagSet()

			words, err := parseInterspersed(fs, tt.args)
			if err != nil {
				t.Fatalf("parseInterspersed: %v", err)
			}
			if !reflect.DeepEqual(words, tt.wantWords) {
				t.Errorf("words = %v, want %v", words, tt.wantWords)
			}
			if *limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", *limit, tt.wantLimit)
			}
			if *asJSON != tt.wantJSON {
				t.Errorf("json = %v, want %v", *asJSON, tt.wantJSON)
			}
			if tt.wantTags != nil && !reflect.DeepEqual([]string(*tags), tt.wantTags) {
				t.Errorf("tags = %v, want %v", *tags, tt.wantTags)
			}
		})
	}
}

func TestParseInterspersedTerminatorStopsFlagParsing(t *testing.T) {
	const dashTerm = "-weird-term"
	for _, args := range [][]string{
		{"--", queryShader, flagJSON, dashTerm},
		{flagSearchLimit, "10", "--", queryShader, flagJSON, dashTerm},
		{queryShader, "--", flagJSON, dashTerm},
	} {
		fs, limit, asJSON, _ := newTestFlagSet()
		words, err := parseInterspersed(fs, args)
		if err != nil {
			t.Fatalf("parseInterspersed(%v): %v", args, err)
		}
		want := []string{queryShader, flagJSON, dashTerm}
		if !reflect.DeepEqual(words, want) {
			t.Errorf("words = %v, want %v", words, want)
		}
		if *limit != 10 || *asJSON {
			t.Errorf("flags after terminator parsed: limit=%d, json=%v", *limit, *asJSON)
		}
	}
}

func TestParseInterspersedUnknownFlagErrors(t *testing.T) {
	fs, _, _, _ := newTestFlagSet()

	if _, err := parseInterspersed(fs, []string{queryShader, "--nope"}); err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
}

func TestExitCodeForFlagErr(t *testing.T) {
	if got := exitCodeForFlagErr(flag.ErrHelp); got != 0 {
		t.Errorf("--help exit code = %d, want 0", got)
	}
	if got := exitCodeForFlagErr(errors.New("bad flag")); got != 2 {
		t.Errorf("bad flag exit code = %d, want 2", got)
	}
}

func TestStringListSet(t *testing.T) {
	var s stringList
	if err := s.Set("go, cli ,,tui"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"go", "cli", "tui"}
	if !reflect.DeepEqual([]string(s), want) {
		t.Errorf("stringList = %v, want %v", s, want)
	}
	if s.String() != "go,cli,tui" {
		t.Errorf("String() = %q", s.String())
	}
}

func TestIsReadOnlyCommand(t *testing.T) {
	readOnly := []string{commandSearch, commandShow, commandTags, commandStats}
	writes := []string{commandImport, commandAdd, commandClean, commandReset, commandWeb, testBogus, ""}

	for _, c := range readOnly {
		if !isReadOnlyCommand(c) {
			t.Errorf("%q should be read-only", c)
		}
	}
	for _, c := range writes {
		if isReadOnlyCommand(c) {
			t.Errorf("%q should not be read-only", c)
		}
	}
}

func TestIsWriteCommand(t *testing.T) {
	for _, command := range []string{commandImport, commandAdd, commandClean, commandReset, commandWeb} {
		if !isWriteCommand(command) {
			t.Errorf("%q should be a write command", command)
		}
	}
	for _, command := range []string{commandSearch, commandShow, commandTags, commandStats, testBogus, ""} {
		if isWriteCommand(command) {
			t.Errorf("%q should not be a write command", command)
		}
	}
}
