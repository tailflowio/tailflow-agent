package parser

func (s *ParserTestSuite) TestParseBytes_WithTesting() {
	yaml := `
version: "2.0"
name: "test-wf"
stages:
  - name: default
steps:
  - id: step1
    stage: default
    action: log
    config:
      message: "hello"
    testing:
      - name: "happy-path"
        output:
          rows:
            - id: 42
      - name: "db-error"
        error:
          message: "connection refused"
      - name: "check-result"
        expect:
          output:
            result: 99
          status: "success"
`
	w, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Len(w.Steps[0].Testing, 3)

	tc0 := w.Steps[0].Testing[0]
	s.Equal("happy-path", tc0.Name)
	s.NotNil(tc0.Output)
	s.Nil(tc0.Error)
	s.Nil(tc0.Expect)

	tc1 := w.Steps[0].Testing[1]
	s.Equal("db-error", tc1.Name)
	s.Nil(tc1.Output)
	s.Require().NotNil(tc1.Error)
	s.Equal("connection refused", tc1.Error.Message)

	tc2 := w.Steps[0].Testing[2]
	s.Equal("check-result", tc2.Name)
	s.Nil(tc2.Output)
	s.Nil(tc2.Error)
	s.Require().NotNil(tc2.Expect)
	s.Equal("success", tc2.Expect.Status)
	s.NotNil(tc2.Expect.Output)
}

func (s *ParserTestSuite) TestValidate_TestingNameRequired() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{{
			ID:      "s1",
			Stage:   "default",
			Action:  "log",
			Testing: []TestCase{{Output: map[string]any{"ok": true}}},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have a name")
}

func (s *ParserTestSuite) TestValidate_TestingDuplicateName() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{{
			ID:     "s1",
			Stage:  "default",
			Action: "log",
			Testing: []TestCase{
				{Name: "case1", Output: "a"},
				{Name: "case1", Output: "b"},
			},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "duplicate test case name")
}

func (s *ParserTestSuite) TestValidate_TestingOutputAndError() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{{
			ID:     "s1",
			Stage:  "default",
			Action: "log",
			Testing: []TestCase{
				{Name: "bad", Output: "x", Error: &TestCaseError{Message: "err"}},
			},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "cannot have both output and error")
}

func (s *ParserTestSuite) TestValidate_TestingInvalidExpectStatus() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{{
			ID:     "s1",
			Stage:  "default",
			Action: "log",
			Testing: []TestCase{
				{Name: "bad", Expect: &TestCaseExpect{Status: "unknown"}},
			},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "invalid expect status")
}

func (s *ParserTestSuite) TestValidate_TestingValidExpectStatuses() {
	for _, status := range []string{"success", "failed", "skipped"} {
		w := &Workflow{
			Version: "2.0",
			Name:    "test",
			Stages:  []Stage{{Name: "default"}},
			Steps: []Step{{
				ID:     "s1",
				Stage:  "default",
				Action: "log",
				Testing: []TestCase{
					{Name: "ok", Expect: &TestCaseExpect{Status: status}},
				},
			}},
		}
		err := Validate(w)
		s.NoError(err, "status %q should be valid", status)
	}
}

func (s *ParserTestSuite) TestValidate_TestingValidCases() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{{
			ID:     "s1",
			Stage:  "default",
			Action: "log",
			Testing: []TestCase{
				{Name: "mock-output", Output: map[string]any{"ok": true}},
				{Name: "mock-error", Error: &TestCaseError{Message: "fail"}},
				{Name: "expect-only", Expect: &TestCaseExpect{Status: "success"}},
			},
		}},
	}
	err := Validate(w)
	s.NoError(err)
}
