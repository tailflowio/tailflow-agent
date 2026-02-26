package runtime

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type DBPoolTestSuite struct {
	suite.Suite
}

func TestDBPool(t *testing.T) {
	suite.Run(t, new(DBPoolTestSuite))
}

func (s *DBPoolTestSuite) SetupTest() { // required by convention
}

func (s *DBPoolTestSuite) TestDetectDriver_Postgres() {
	d, err := detectDriver("postgres://user:pass@localhost:5432/db")
	s.NoError(err)
	s.Equal("pgx", d)

	d, err = detectDriver("postgresql://user:pass@localhost:5432/db")
	s.NoError(err)
	s.Equal("pgx", d)
}

func (s *DBPoolTestSuite) TestDetectDriver_MySQL() {
	d, err := detectDriver("user:pass@tcp(localhost:3306)/db")
	s.NoError(err)
	s.Equal("mysql", d)
}

func (s *DBPoolTestSuite) TestDetectDriver_Unknown() {
	_, err := detectDriver("something-else")
	s.Error(err)
	s.Contains(err.Error(), "cannot detect driver")
}

func (s *DBPoolTestSuite) TestMemoryDBPool_CloseEmpty() {
	pool := NewMemoryDBPool()
	err := pool.Close()
	s.NoError(err)
}
