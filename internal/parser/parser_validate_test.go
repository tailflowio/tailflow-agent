package parser

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
