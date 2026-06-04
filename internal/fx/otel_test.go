package fx

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	uberfx "go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

type OTelTestSuite struct {
	suite.Suite
}

func TestOTelTestSuite(t *testing.T) {
	suite.Run(t, new(OTelTestSuite))
}

func (s *OTelTestSuite) TestNewOTel_Success() {
	app := fxtest.New(s.T(),
		uberfx.NopLogger,
		uberfx.Supply(Config{OTel: tfotel.Config{}}),
		uberfx.Provide(NewOTel),
		uberfx.Invoke(func(_ *tfotel.Result) {}),
	)

	app.RequireStart()
	app.RequireStop()
}

func (s *OTelTestSuite) TestNewOTel_WhenSetupFails() {
	original := tfotelSetup
	tfotelSetup = func(_ context.Context, _ tfotel.Config) (*tfotel.Result, error) {
		return nil, errors.New("otel setup failure")
	}

	defer func() { tfotelSetup = original }()

	var result *tfotel.Result

	app := uberfx.New(
		uberfx.NopLogger,
		uberfx.Supply(Config{OTel: tfotel.Config{}}),
		uberfx.Provide(NewOTel),
		uberfx.Populate(&result),
	)

	s.Require().Error(app.Err())
	s.Contains(app.Err().Error(), "otel setup")
}
