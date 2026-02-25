//go:build !saas

package action

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileReadAction reads a file from disk.
type FileReadAction struct{}

func NewFileReadAction() Action { return &FileReadAction{} }

func (a *FileReadAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["path"]; !ok {
		return errors.New("file.read action requires 'path' in config")
	}

	return nil
}

func (a *FileReadAction) Execute(ctx *ActionContext) (any, error) {
	path := fmt.Sprintf("%v", ctx.Config["path"])
	path = filepath.Clean(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("file.read: %w", err)
	}

	return map[string]any{
		"content": string(data),
		"path":    path,
		"size":    len(data),
	}, nil
}

// FileWriteAction writes content to a file.
type FileWriteAction struct{}

func NewFileWriteAction() Action { return &FileWriteAction{} }

func (a *FileWriteAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["path"]; !ok {
		return errors.New("file.write action requires 'path' in config")
	}

	if _, ok := ctx.Config["content"]; !ok {
		return errors.New("file.write action requires 'content' in config")
	}

	return nil
}

func (a *FileWriteAction) Execute(ctx *ActionContext) (any, error) {
	path := fmt.Sprintf("%v", ctx.Config["path"])
	path = filepath.Clean(path)
	content := fmt.Sprintf("%v", ctx.Config["content"])

	perm := os.FileMode(0o644)

	if p, ok := ctx.Config["perm"]; ok {
		if pi, ok := p.(int); ok {
			perm = os.FileMode(pi)
		}
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return nil, fmt.Errorf("file.write: create dir: %w", err)
	}

	err = os.WriteFile(path, []byte(content), perm)
	if err != nil {
		return nil, fmt.Errorf("file.write: %w", err)
	}

	return map[string]any{
		"path": path,
		"size": len(content),
	}, nil
}
