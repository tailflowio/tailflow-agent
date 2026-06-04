package parser

func (s *ParserTestSuite) TestValidate_PersistenceOmittedIsValid() {
	w := newValidPersistenceWorkflow()
	w.Persistence = nil
	s.NoError(Validate(w))
}

func (s *ParserTestSuite) TestValidate_PersistenceMemoryDefault() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{}
	s.NoError(Validate(w))
}

func (s *ParserTestSuite) TestValidate_PersistenceMemoryWithMaxExecutions() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:   PersistenceMemory,
		Memory: &MemoryPersistence{MaxExecutions: 50},
	}
	s.NoError(Validate(w))
}

func (s *ParserTestSuite) TestValidate_PersistenceMemoryNegativeMaxRejected() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:   PersistenceMemory,
		Memory: &MemoryPersistence{MaxExecutions: -1},
	}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "max_executions")
}

func (s *ParserTestSuite) TestValidate_PersistenceMariaDB() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:    PersistenceMariaDB,
		MariaDB: &MariaDBPersistence{DSN: "user:pass@tcp(localhost:3306)/tailflow"},
	}
	s.NoError(Validate(w))
}

func (s *ParserTestSuite) TestValidate_PersistenceMariaDBMissingSubBlock() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{Type: PersistenceMariaDB}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "persistence.mariadb")
}

func (s *ParserTestSuite) TestValidate_PersistenceMariaDBMissingDSN() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:    PersistenceMariaDB,
		MariaDB: &MariaDBPersistence{},
	}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "dsn is required")
}

func (s *ParserTestSuite) TestValidate_PersistenceUnknownType() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{Type: "redis"}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "not supported")
}

func (s *ParserTestSuite) TestValidate_PersistenceMemoryTypeWithMariaDBSubBlockRejected() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:    PersistenceMemory,
		MariaDB: &MariaDBPersistence{DSN: "user@/db"},
	}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "must not declare other backend sub-blocks")
}

func (s *ParserTestSuite) TestValidate_PersistenceDefaultTypeWithMariaDBSubBlockRejected() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:    "",
		MariaDB: &MariaDBPersistence{DSN: "user@/db"},
	}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "must not declare other backend sub-blocks")
}

func (s *ParserTestSuite) TestValidate_PersistenceMariaDBWithExtraSubBlockRejected() {
	w := newValidPersistenceWorkflow()
	w.Persistence = &Persistence{
		Type:    PersistenceMariaDB,
		MariaDB: &MariaDBPersistence{DSN: "user@/db"},
		Memory:  &MemoryPersistence{MaxExecutions: 1},
	}
	err := Validate(w)
	s.Require().Error(err)
	s.Contains(err.Error(), "must not declare other backend sub-blocks")
}

func (s *ParserTestSuite) TestParseBytes_PersistenceMariaDBYAML() {
	yaml := `
version: "2.0"
name: "test"
persistence:
  type: mariadb
  mariadb:
    dsn: "user:pass@tcp(localhost:3306)/tailflow"
    table_prefix: "tf_"
stages:
  - name: default
steps:
  - id: s1
    stage: default
    action: log
`
	w, err := ParseBytes([]byte(yaml))
	s.Require().NoError(err)
	s.Require().NotNil(w.Persistence)
	s.Equal("mariadb", w.Persistence.Type)
	s.Require().NotNil(w.Persistence.MariaDB)
	s.Equal("user:pass@tcp(localhost:3306)/tailflow", w.Persistence.MariaDB.DSN)
	s.Equal("tf_", w.Persistence.MariaDB.TablePrefix)
}
