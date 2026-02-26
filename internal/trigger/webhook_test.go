package trigger

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/suite"
)

type WebhookTestSuite struct {
	suite.Suite
}

func TestWebhook(t *testing.T) {
	suite.Run(t, new(WebhookTestSuite))
}

func (s *WebhookTestSuite) SetupTest() {
	// required by convention
}

func (s *WebhookTestSuite) TestValidateWebhookSignature_Valid() {
	secret := "mysecret"
	payload := []byte(`{"action":"push"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	err := ValidateWebhookSignature(payload, secret, sig)
	s.NoError(err)
}

func (s *WebhookTestSuite) TestValidateWebhookSignature_WithPrefix() {
	secret := "mysecret"
	payload := []byte(`{"action":"push"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	err := ValidateWebhookSignature(payload, secret, sig)
	s.NoError(err)
}

func (s *WebhookTestSuite) TestValidateWebhookSignature_Invalid() {
	err := ValidateWebhookSignature([]byte("data"), "secret", "invalidsig")
	s.Error(err)
}

func (s *WebhookTestSuite) TestValidateWebhookSignature_NoSecret() {
	err := ValidateWebhookSignature([]byte("data"), "", "")
	s.NoError(err)
}

func (s *WebhookTestSuite) TestValidateWebhookSignature_MissingSig() {
	err := ValidateWebhookSignature([]byte("data"), "secret", "")
	s.Error(err)
}
