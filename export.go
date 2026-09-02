package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ExportSkill saves the local SKILL.md directory as a ZIP-compatible archive.
// An empty result means the user cancelled the save dialog.
func (a *App) ExportSkill(skillID, format string) (string, error) {
	if format != "zip" && format != "skill" {
		return "", errors.New("unsupported archive format")
	}
	a.mu.Lock()
	skill, err := a.findSkillLocked(skillID)
	ctx := a.ctx
	a.mu.Unlock()
	if err != nil {
		return "", err
	}
	if skill.SourcePath == "" {
		return "", errors.New("skill source path is unavailable")
	}
	root, err := exportSkillRoot(skill.SourcePath)
	if err != nil {
		return "", err
	}
	destination, err := wailsRuntime.SaveFileDialog(ctx, wailsRuntime.SaveDialogOptions{
		Title:                "导出 Skill 为 ." + format,
		DefaultFilename:      filepath.Base(filepath.Clean(skill.SourcePath)) + "." + format,
		Filters:              []wailsRuntime.FileFilter{{DisplayName: "Skill archive (." + format + ")", Pattern: "*." + format}},
		CanCreateDirectories: true,
	})
	if err != nil || destination == "" {
		return "", err
	}
	// The native dialog supplies the selected extension; do not silently change
	// the destination after its overwrite confirmation.
	if !strings.EqualFold(filepath.Ext(destination), "."+format) {
		return "", fmt.Errorf("please save the archive with a .%s extension", format)
	}
	if err := writeSkillArchive(root, filepath.Base(filepath.Clean(skill.SourcePath)), destination); err != nil {
		return "", err
	}
	return destination, nil
}

func exportSkillRoot(source string) (string, error) {
	root, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(filepath.Join(root, "SKILL.md"))
	if err != nil {
		return "", fmt.Errorf("cannot export skill: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("SKILL.md must be a regular file")
	}
	return root, nil
}

func writeSkillArchive(root, name, destination string) error {
	// Resolve the parent so a symlink cannot put the output inside the source.
	parent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return err
	}
	destination = filepath.Join(parent, filepath.Base(destination))
	rel, err := filepath.Rel(root, destination)
	if err != nil {
		return err
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("choose a save location outside the skill directory")
	}
	temp, err := os.CreateTemp(parent, ".skill-export-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	archive := zip.NewWriter(temp)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("cannot archive special file: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(name, relative))
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Preserve symlinks without reading data outside the selected directory.
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_, err = io.WriteString(writer, target)
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		return errors.Join(copyErr, closeErr)
	})
	closeErr := archive.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// Publish only a complete archive, leaving any previous export intact on error.
	return os.Rename(temp.Name(), destination)
}
