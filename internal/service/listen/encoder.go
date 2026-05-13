package listen

import "encoding/base64"

// Encoder encodes raw bytes to a string representation.
type Encoder interface {
	Encode(data []byte) (string, error)
}

// Base64Encoder encodes bytes using standard base64 encoding.
type Base64Encoder struct{}

func (e *Base64Encoder) Encode(data []byte) (string, error) {
	return base64.StdEncoding.EncodeToString(data), nil
}
