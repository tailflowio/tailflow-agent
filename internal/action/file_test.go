//go:build !saas

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
