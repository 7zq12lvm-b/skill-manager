package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillArchiveRoundTrip(t *testing.T) {
	for _, extension := range []string{"zip", "skill"} {
		t.Run(extension, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "示例-skill")
			if err := os.MkdirAll(filepath.Join(root, "scripts", "empty"), 0755); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{"SKILL.md": "---\nname: example\n---\n", "scripts/run.sh": "#!/bin/sh\necho ok\n", ".hidden": "hidden", "data.bin": "\x00\xff\x01"}
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink("SKILL.md", filepath.Join(root, "manifest-link")); err != nil {
				t.Fatal(err)
			}
			resolved, err := exportSkillRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(parent, "示例-skill."+extension)
			if err := writeSkillArchive(resolved, filepath.Base(root), dest); err != nil {
				t.Fatal(err)
			}
			archive, err := zip.OpenReader(dest)
			if err != nil {
				t.Fatal(err)
			}
			defer archive.Close()
			entries := map[string]*zip.File{}
			for _, file := range archive.File {
				entries[file.Name] = file
			}
			for name, expected := range files {
				file := entries["示例-skill/"+name]
				if file == nil {
					t.Fatalf("missing %s", name)
				}
				reader, err := file.Open()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(reader)
				reader.Close()
				if err != nil || string(data) != expected {
					t.Fatalf("bad contents: %s: %v", name, err)
				}
			}
			if entries["示例-skill/scripts/empty/"] == nil {
				t.Fatal("empty directory missing")
			}
			if entries["示例-skill/scripts/run.sh"].Mode().Perm() != 0755 {
				t.Fatal("executable mode lost")
			}
			if entries["示例-skill/manifest-link"].Mode()&os.ModeSymlink == 0 {
				t.Fatal("symlink mode lost")
			}
		})
	}
}

func TestSkillArchiveRejectsOutputInsideSource(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	resolved, err := exportSkillRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeSkillArchive(resolved, "skill", filepath.Join(root, "SKILL.md")); err == nil {
		t.Fatal("expected error")
	}
	data, _ := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if string(data) != "original" {
		t.Fatal("source overwritten")
	}
}

func TestSkillExportRequiresManifest(t *testing.T) {
	if _, err := exportSkillRoot(t.TempDir()); err == nil {
		t.Fatal("expected missing manifest error")
	}
}
