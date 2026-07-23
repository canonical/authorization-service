// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import (
	"errors"
	"fmt"
)

// PermanentError marks a failure that will never succeed on retry (such as
// undecodable payloads, service/topic mismatches, or operations rejected by OpenFGA).
type PermanentError struct {
	Code string
	Err  error
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}

// NewPermanentError constructs a PermanentError.
func NewPermanentError(code string, err error) *PermanentError {
	return &PermanentError{Code: code, Err: err}
}

// IsPermanent reports whether err is or wraps a PermanentError.
func IsPermanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}

// AsPermanent attempts to retrieve the underlying PermanentError from err.
func AsPermanent(err error) (*PermanentError, bool) {
	var p *PermanentError
	if errors.As(err, &p) {
		return p, true
	}
	return nil, false
}
