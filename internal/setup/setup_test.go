package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateClaudeMd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "CLAUDE.md")

	// Created when absent.
	action, err := UpdateClaudeMd(path)
	if err != nil || action != "created" {
		t.Fatalf("create action=%q err=%v", action, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), claudeMdStart) || !strings.Contains(string(data), claudeMdEnd) {
		t.Fatalf("markers missing:\n%s", data)
	}

	// Idempotent second run.
	if action, err = UpdateClaudeMd(path); err != nil || action != "unchanged" {
		t.Fatalf("second run action=%q err=%v", action, err)
	}

	// Existing user content is preserved and block appended once.
	userDoc := "# My rules\n\nsome guidance\n"
	if err := os.WriteFile(path, []byte(userDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	if action, err = UpdateClaudeMd(path); err != nil || action != "updated" {
		t.Fatalf("append action=%q err=%v", action, err)
	}
	got, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(got), userDoc) {
		t.Errorf("user content clobbered:\n%s", got)
	}
	if n := strings.Count(string(got), claudeMdStart); n != 1 {
		t.Errorf("block count = %d, want 1", n)
	}

	// A stale block body is replaced in place, not duplicated.
	stale := "prefix\n" + claudeMdStart + "\nOLD BODY\n" + claudeMdEnd + "\nsuffix\n"
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	if action, err = UpdateClaudeMd(path); err != nil || action != "updated" {
		t.Fatalf("replace action=%q err=%v", action, err)
	}
	got, _ = os.ReadFile(path)
	str := string(got)
	if strings.Contains(str, "OLD BODY") {
		t.Errorf("stale body not replaced:\n%s", str)
	}
	if !strings.HasPrefix(str, "prefix\n") || !strings.HasSuffix(str, "suffix\n") {
		t.Errorf("surrounding content not preserved:\n%s", str)
	}
	if n := strings.Count(str, claudeMdStart); n != 1 {
		t.Errorf("block count after replace = %d, want 1", n)
	}
}
