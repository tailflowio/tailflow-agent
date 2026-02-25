package action

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// HashAction computes a SHA-256 hash of the given input string.
type HashAction struct{}

func NewHashAction() Action { return &HashAction{} }

func (a *HashAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["input"]; !ok {
		return errors.New("hash action requires 'input' in config")
	}

	return nil
}

func (a *HashAction) Execute(ctx *ActionContext) (any, error) {
	input := fmt.Sprintf("%v", ctx.Config["input"])

	sum := sha256.Sum256([]byte(input))
	hash := hex.EncodeToString(sum[:])

	return map[string]any{
		"hash": hash,
	}, nil
}
