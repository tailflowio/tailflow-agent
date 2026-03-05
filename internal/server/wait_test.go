package server

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/runtime"
)

type WaitRegistryTestSuite struct {
	suite.Suite
	registry *WaitRegistry
}

func TestWaitRegistry(t *testing.T) {
	suite.Run(t, new(WaitRegistryTestSuite))
}

func (s *WaitRegistryTestSuite) SetupTest() {
	s.registry = NewWaitRegistry()
}

func (s *WaitRegistryTestSuite) TestRegister_CreatesRegistration() {
	ch, cleanup := s.registry.Register("exec-1", "step-1", "/hook", context.Background())

	s.NotNil(ch)
	s.NotNil(cleanup)
}

func (s *WaitRegistryTestSuite) TestDeliver_SendsRequestToWaiter() {
	ch, _ := s.registry.Register("exec-1", "step-1", "/hook", context.Background())

	req := runtime.WaitRequest{
		Method: "POST",
		Path:   "/hook",
		Body:   "payload",
	}

	err := s.registry.Deliver("exec-1", "/hook", req)
	s.Require().NoError(err)

	received := <-ch
	s.Equal("POST", received.Method)
	s.Equal("/hook", received.Path)
	s.Equal("payload", received.Body)
}

func (s *WaitRegistryTestSuite) TestDeliver_ReturnsError_WhenNotFound() {
	err := s.registry.Deliver("unknown", "/hook", runtime.WaitRequest{})

	s.Error(err)
	s.Contains(err.Error(), "no wait registration")
}

func (s *WaitRegistryTestSuite) TestCleanup_RemovesRegistration() {
	_, cleanup := s.registry.Register("exec-1", "step-1", "/hook", context.Background())

	cleanup()

	err := s.registry.Deliver("exec-1", "/hook", runtime.WaitRequest{})
	s.Error(err)
	s.Contains(err.Error(), "no wait registration")
}

func (s *WaitRegistryTestSuite) TestRegister_ContextCancellation() {
	synctest.Test(s.T(), func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		registry := NewWaitRegistry()
		_, _ = registry.Register("exec-1", "step-1", "/hook", ctx)

		cancel()
		synctest.Wait()

		err := registry.Deliver("exec-1", "/hook", runtime.WaitRequest{})
		if err == nil {
			t.Fatal("expected error after context cancellation, got nil")
		}
	})
}

func (s *WaitRegistryTestSuite) TestMultipleRegistrations() {
	ch1, _ := s.registry.Register("exec-1", "step-1", "/hook-a", context.Background())
	ch2, _ := s.registry.Register("exec-2", "step-2", "/hook-b", context.Background())

	reqA := runtime.WaitRequest{Method: "GET", Path: "/hook-a"}
	reqB := runtime.WaitRequest{Method: "POST", Path: "/hook-b"}

	err := s.registry.Deliver("exec-1", "/hook-a", reqA)
	s.Require().NoError(err)

	err = s.registry.Deliver("exec-2", "/hook-b", reqB)
	s.Require().NoError(err)

	receivedA := <-ch1
	s.Equal("GET", receivedA.Method)

	receivedB := <-ch2
	s.Equal("POST", receivedB.Method)
}

func (s *WaitRegistryTestSuite) TestDeliver_ChannelFull_ReturnsError() {
	ch, _ := s.registry.Register("exec-1", "step-1", "/hook", context.Background())

	// Fill the channel (buffer size is 1)
	err := s.registry.Deliver("exec-1", "/hook", runtime.WaitRequest{Method: "POST"})
	s.Require().NoError(err)

	// Second deliver should fail because channel is full
	err = s.registry.Deliver("exec-1", "/hook", runtime.WaitRequest{Method: "POST"})
	s.Require().Error(err)
	s.Contains(err.Error(), "wait channel full")

	// Drain the channel
	<-ch
}

func (s *WaitRegistryTestSuite) TestRegister_NilContext_NoAutoCleanup() {
	ch, cleanup := s.registry.Register("exec-1", "step-1", "/hook", nil)
	defer cleanup()

	s.NotNil(ch)

	// Should still be deliverable
	err := s.registry.Deliver("exec-1", "/hook", runtime.WaitRequest{Method: "GET"})
	s.NoError(err)
	<-ch
}
