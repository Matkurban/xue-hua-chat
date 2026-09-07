package admin

import (
	"errors"
	"testing"
)

type wrapErr struct {
	inner error
}

func (e wrapErr) Error() string { return "wrap: " + e.inner.Error() }
func (e wrapErr) Unwrap() error { return e.inner }

type textErr string

func (e textErr) Error() string { return string(e) }

func TestIsDupKey(t *testing.T) {
	if isDupKey(nil) {
		t.Fatal("nil is not duplicate")
	}
	if isDupKey(textErr("connection refused")) {
		t.Fatal("network error is not duplicate")
	}
	if !isDupKey(textErr("E11000 duplicate key error collection")) {
		t.Fatal("E11000 should count as duplicate")
	}
	if !isDupKey(wrapErr{inner: textErr("write E11000 duplicate key")}) {
		t.Fatal("wrapped E11000 should count as duplicate")
	}
}

func TestIsDupKeyIgnoresUnrelated(t *testing.T) {
	if isDupKey(errors.New("timeout")) {
		t.Fatal("timeout is not duplicate")
	}
}
