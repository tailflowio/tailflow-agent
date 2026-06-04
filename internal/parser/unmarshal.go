package parser

import "gopkg.in/yaml.v3"

func (s *Step) UnmarshalYAML(value *yaml.Node) error {
	type rawStep Step

	var raw rawStep

	decodeErr := value.Decode(&raw)
	if decodeErr != nil {
		return decodeErr
	}

	*s = Step(raw)
	s.Line = value.Line

	return nil
}

func (s *Stage) UnmarshalYAML(value *yaml.Node) error {
	type rawStage Stage

	var raw rawStage

	decodeErr := value.Decode(&raw)
	if decodeErr != nil {
		return decodeErr
	}

	*s = Stage(raw)
	s.Line = value.Line

	return nil
}

func (p *Param) UnmarshalYAML(value *yaml.Node) error {
	type rawParam Param

	var raw rawParam

	decodeErr := value.Decode(&raw)
	if decodeErr != nil {
		return decodeErr
	}

	*p = Param(raw)
	p.Line = value.Line

	return nil
}
