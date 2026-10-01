package main

import (
	"errors"
	"testing"
)

func TestRecoveryFailureNeverAutoRestartsWriter(t *testing.T) {
	cause := errors.New("fixture recovery failure")
	err := errors.Join(errRecoveryBlocked, cause)
	if exitCode(err) != 78 {
		t.Fatal("recovery failure would restart helper and stop preview again")
	}
	if !errors.Is(err, cause) {
		t.Fatal("recovery cause lost")
	}
	if exitCode(errors.New("fixture listener failure")) != 1 {
		t.Fatal("ordinary failure classification changed")
	}
}
