package trigger

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

func ValidateWebhookSignature(payload []byte, secret, signature string) error {
	if secret == "" {
		return nil
	}

	if signature == "" {
		return errors.New("missing webhook signature")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	// Support "sha256=..." prefix (GitHub style)
	sig := signature
	if len(sig) > 7 && sig[:7] == "sha256=" {
		sig = sig[7:]
	}

	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return errors.New("invalid webhook signature")
	}

	return nil
}
