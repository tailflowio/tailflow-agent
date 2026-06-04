package parser

import "errors"

func (s *ParserTestSuite) TestValidate_MissingVersion() {
	w := &Workflow{Name: "test", Steps: []Step{{ID: "s1", Action: "log"}}}
	err := Validate(w)
	s.ErrorContains(err, "version is required")
}

func (s *ParserTestSuite) TestValidate_UnsupportedVersion() {
	w := &Workflow{Version: "1.0", Name: "test", Steps: []Step{{ID: "s1", Action: "log"}}}
	err := Validate(w)
	s.ErrorContains(err, "unsupported version")
}

func (s *ParserTestSuite) TestValidate_MissingName() {
	w := &Workflow{Version: "2.0", Steps: []Step{{ID: "s1", Action: "log"}}}
	err := Validate(w)
	s.ErrorContains(err, "name is required")
}

func (s *ParserTestSuite) TestValidate_NoSteps() {
	w := &Workflow{Version: "2.0", Name: "test"}
	err := Validate(w)
	s.ErrorContains(err, "at least one step")
}

func (s *ParserTestSuite) TestValidate_MissingStepID() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have an id")
}

func (s *ParserTestSuite) TestValidate_DuplicateStepID() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{
			{ID: "s1", Stage: "default", Action: "log"},
			{ID: "s1", Stage: "default", Action: "exec"},
		},
	}
	err := Validate(w)
	s.ErrorContains(err, "duplicate step id")
}

func (s *ParserTestSuite) TestValidate_MissingAction() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have an action")
}

func (s *ParserTestSuite) TestValidate_UnknownDependency() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{
			{ID: "s1", Stage: "default", Action: "log", DependsOn: []string{"nonexistent"}},
		},
	}
	err := Validate(w)
	s.ErrorContains(err, "unknown step")
}

func (s *ParserTestSuite) TestValidate_SelfDependency() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{
			{ID: "s1", Stage: "default", Action: "log", DependsOn: []string{"s1"}},
		},
	}
	err := Validate(w)
	s.ErrorContains(err, "cannot depend on itself")
}

func (s *ParserTestSuite) TestValidate_InvalidParamType() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Params:  []Param{{Name: "p", Type: "unknown"}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "unsupported type")
}

func (s *ParserTestSuite) TestValidate_ErrorPolicyValid() {
	for _, policy := range []string{"", "stop", "continue", "ignore"} {
		w := &Workflow{
			Version: "2.0",
			Name:    "test",
			Stages:  []Stage{{Name: "default"}},
			Steps: []Step{
				{ID: "s1", Stage: "default", Action: "log", ErrorPolicy: policy},
			},
		}
		err := Validate(w)
		s.NoError(err, "policy %q should be valid", policy)
	}
}

func (s *ParserTestSuite) TestValidate_ErrorPolicyInvalid() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{
			{ID: "s1", Stage: "default", Action: "log", ErrorPolicy: "retry"},
		},
	}
	err := Validate(w)
	s.ErrorContains(err, "invalid error_policy")
}

func (s *ParserTestSuite) TestValidate_ParamMissingName() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Params:  []Param{{Type: "string"}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have a name")
}

func (s *ParserTestSuite) TestValidate_ParamMissingType() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Params:  []Param{{Name: "p"}},
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have a type")
}

func (s *ParserTestSuite) TestValidate_OnError_MissingID() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{{
			ID:      "s1",
			Stage:   "default",
			Action:  "log",
			OnError: []Step{{Action: "log"}},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "on_error")
	s.ErrorContains(err, "must have an id")
}

func (s *ParserTestSuite) TestValidate_OnError_MissingAction() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps: []Step{{
			ID:      "s1",
			Stage:   "default",
			Action:  "log",
			OnError: []Step{{ID: "err1"}},
		}},
	}
	err := Validate(w)
	s.ErrorContains(err, "on_error")
	s.ErrorContains(err, "must have an action")
}

func (s *ParserTestSuite) TestValidate_WorkflowOnError_MissingID() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
		OnError: []Step{{Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "workflow on_error")
	s.ErrorContains(err, "must have an id")
}

func (s *ParserTestSuite) TestValidate_WorkflowOnError_MissingAction() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
		OnError: []Step{{ID: "err1"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "workflow on_error")
	s.ErrorContains(err, "must have an action")
}

func (s *ParserTestSuite) TestValidate_NoStages() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "at least one stage is required")
}

func (s *ParserTestSuite) TestValidate_StageWithNoName() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: ""}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have a name")
}

func (s *ParserTestSuite) TestValidate_DuplicateStageName() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}, {Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "duplicate stage name")
}

func (s *ParserTestSuite) TestValidate_StepWithNoStage() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "must have a stage")
}

func (s *ParserTestSuite) TestValidate_StepReferencesUnknownStage() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "unknown", Action: "log"}},
	}
	err := Validate(w)
	s.ErrorContains(err, "references unknown stage")
}

func (s *ParserTestSuite) TestValidate_AccumulatesMultipleErrors() {
	w := &Workflow{
		Version: "1.0",
		Steps: []Step{
			{ID: "a", Action: "log"},
		},
	}
	err := Validate(w)
	messages := Messages(err)

	s.GreaterOrEqual(len(messages), 4)
	s.ErrorContains(err, "unsupported version")
	s.ErrorContains(err, "name is required")
	s.ErrorContains(err, "at least one stage is required")
	s.ErrorContains(err, "must have a stage")
}

func (s *ParserTestSuite) TestValidate_DuplicateStepAndParamErrorsBothReported() {
	w := &Workflow{
		Version: "2.0",
		Name:    "test",
		Params: []Param{
			{Name: "p1", Type: "bad"},
			{Type: "string"},
		},
		Stages: []Stage{{Name: "default"}, {Name: "default"}, {Name: ""}},
		Steps: []Step{
			{ID: "s1", Stage: "default", Action: "log"},
			{ID: "s1", Stage: "default", Action: "exec"},
		},
	}
	err := Validate(w)

	s.ErrorContains(err, "unsupported type")
	s.ErrorContains(err, "param[1] must have a name")
	s.ErrorContains(err, "duplicate stage name")
	s.ErrorContains(err, "must have a name")
	s.ErrorContains(err, "duplicate step id")
}

func (s *ParserTestSuite) TestValidate_ValidVersionMissingNameDoesNotReportVersionError() {
	w := &Workflow{
		Version: "2.0",
		Stages:  []Stage{{Name: "default"}},
		Steps:   []Step{{ID: "s1", Stage: "default", Action: "log"}},
	}
	err := Validate(w)

	s.ErrorContains(err, "name is required")
	s.NotContains(err.Error(), "unsupported version")
}

func (s *ParserTestSuite) TestMessages_Nil() {
	s.Nil(Messages(nil))
}

func (s *ParserTestSuite) TestMessages_SingleError() {
	messages := Messages(errors.New("boom"))

	s.Equal([]string{"boom"}, messages)
}

func (s *ParserTestSuite) TestMessages_JoinedErrors() {
	joined := errors.Join(errors.New("first"), errors.New("second"))
	messages := Messages(joined)

	s.Equal([]string{"first", "second"}, messages)
}
