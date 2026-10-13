package errors

import (
	"fmt"
	"testing"
)

func check(fail bool) Error {
	if fail {
		return NewError("Check", "fail is true", fmt.Errorf("underlying error"))
	}
	return nil
}

func TestNewError(t *testing.T) {
	var err error

	err = check(true)
	if expected := "Check: fail is true: underlying error"; err == nil || err.Error() != expected {
		t.Errorf("check(true): got %v, want %v", err, expected)
	}

	err = check(false)
	if err != nil {
		t.Errorf("check(false): got %v, want nil", err)
	}
}
