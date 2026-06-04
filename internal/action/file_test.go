package action

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type FileActionTestSuite struct {
	suite.Suite
}

func TestFileAction(t *testing.T) {
	suite.Run(t, new(FileActionTestSuite))
}

func (s *FileActionTestSuite) SetupTest() {}

func (s *FileActionTestSuite) TestWriteAndRead() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "test.txt")

	// Write
	wa := NewFileWriteAction()
	wctx := newTestContext(map[string]any{
		"path":    path,
		"content": "hello world",
	})
	out, err := wa.Execute(wctx)
	s.Require().NoError(err)
	s.Equal(11, out.(map[string]any)["size"])

	// Read
	ra := NewFileReadAction()
	rctx := newTestContext(map[string]any{"path": path})
	out, err = ra.Execute(rctx)
	s.Require().NoError(err)
	s.Equal("hello world", out.(map[string]any)["content"])
}

func (s *FileActionTestSuite) TestReadNotFound() {
	a := NewFileReadAction()
	ctx := newTestContext(map[string]any{"path": "/nonexistent/file.txt"})
	_, err := a.Execute(ctx)
	s.Error(err)
}

func (s *FileActionTestSuite) TestWriteCreatesDir() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "sub", "dir", "test.txt")

	a := NewFileWriteAction()
	ctx := newTestContext(map[string]any{
		"path":    path,
		"content": "nested",
	})
	_, err := a.Execute(ctx)
	s.Require().NoError(err)

	data, err := os.ReadFile(path)
	s.Require().NoError(err)
	s.Equal("nested", string(data))
}

func (s *FileActionTestSuite) TestReadValidateMissingPath() {
	a := NewFileReadAction()
	err := a.Validate(newTestContext(map[string]any{}))
	s.Error(err)
}

func (s *FileActionTestSuite) TestWriteValidateMissingContent() {
	a := NewFileWriteAction()
	err := a.Validate(newTestContext(map[string]any{"path": "/tmp/test"}))
	s.Error(err)
}

func (s *FileActionTestSuite) TestReadValidateOK() {
	a := NewFileReadAction()
	err := a.Validate(newTestContext(map[string]any{"path": "/tmp/test.txt"}))
	s.NoError(err)
}

func (s *FileActionTestSuite) TestWriteValidateOK() {
	a := NewFileWriteAction()
	err := a.Validate(newTestContext(map[string]any{"path": "/tmp/test.txt", "content": "data"}))
	s.NoError(err)
}

func (s *FileActionTestSuite) TestWriteValidateMissingPath() {
	a := NewFileWriteAction()
	err := a.Validate(newTestContext(map[string]any{"content": "data"}))
	s.Error(err)
	s.Contains(err.Error(), "path")
}

func (s *FileActionTestSuite) TestWriteWithPerm() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "perm.txt")

	a := NewFileWriteAction()
	ctx := newTestContext(map[string]any{
		"path":    path,
		"content": "hello",
		"perm":    0o600,
	})
	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(5, out.(map[string]any)["size"])

	info, err := os.Stat(path)
	s.Require().NoError(err)
	s.Equal(os.FileMode(0o600), info.Mode().Perm())
}

func (s *FileActionTestSuite) TestWriteWithNonIntPerm() {
	dir := s.T().TempDir()
	path := filepath.Join(dir, "nonintperm.txt")

	a := NewFileWriteAction()
	ctx := newTestContext(map[string]any{
		"path":    path,
		"content": "hello",
		"perm":    "not-an-int",
	})
	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	s.Equal(5, out.(map[string]any)["size"])
}

func (s *FileActionTestSuite) TestWriteInvalidDir() {
	// Writing to a path where the parent is an existing file (not a directory)
	dir := s.T().TempDir()
	filePath := filepath.Join(dir, "afile")
	err := os.WriteFile(filePath, []byte("x"), 0o644)
	s.Require().NoError(err)

	badPath := filepath.Join(filePath, "subdir", "test.txt")
	a := NewFileWriteAction()
	ctx := newTestContext(map[string]any{
		"path":    badPath,
		"content": "data",
	})
	_, err = a.Execute(ctx)
	s.Error(err)
}

func (s *FileActionTestSuite) TestWriteFilePermissionError() {
	dir := s.T().TempDir()
	readonlyDir := filepath.Join(dir, "readonly")
	err := os.Mkdir(readonlyDir, 0o555)
	s.Require().NoError(err)
	defer os.Chmod(readonlyDir, 0o755) // cleanup

	a := NewFileWriteAction()
	ctx := newTestContext(map[string]any{
		"path":    filepath.Join(readonlyDir, "test.txt"),
		"content": "data",
	})
	_, err = a.Execute(ctx)
	s.Error(err)
}
