package fx

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	tfotel "github.com/tailflow/tailflow/internal/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type BusinessMetricsTestSuite struct {
	suite.Suite
}

func TestBusinessMetricsTestSuite(t *testing.T) {
	suite.Run(t, new(BusinessMetricsTestSuite))
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_Success() {
	mp := sdkmetric.NewMeterProvider()

	out, err := NewBusinessMetrics(BusinessMetricsIn{
		Result: &tfotel.Result{MeterProvider: mp},
	})

	s.Require().NoError(err)
	s.NotNil(out.BusinessMetrics)
}

func (s *BusinessMetricsTestSuite) TestNewBusinessMetrics_WhenCreationFails() {
	original := tfotelNewBusinessMetrics
	tfotelNewBusinessMetrics = func(_ *sdkmetric.MeterProvider) (*tfotel.BusinessMetrics, error) {
		return nil, errors.New("meter error")
	}

	defer func() { tfotelNewBusinessMetrics = original }()

	mp := sdkmetric.NewMeterProvider()

	_, err := NewBusinessMetrics(BusinessMetricsIn{
		Result: &tfotel.Result{MeterProvider: mp},
	})

	s.Require().Error(err)
	s.Contains(err.Error(), "otel business metrics")
}
