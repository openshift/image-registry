package imagestream

import (
	"testing"

	rerrors "github.com/openshift/image-registry/pkg/errors"
)

func TestConvertImageStreamGetterError(t *testing.T) {
	tests := []struct {
		name         string
		inputCode    string
		expectedCode string
	}{
		{
			name:         "NotFound error is propagated as ImageStream NotFound",
			inputCode:    ErrImageStreamGetterNotFoundCode,
			expectedCode: ErrImageStreamNotFoundCode,
		},
		{
			name:         "Forbidden error is propagated as ImageStream Forbidden",
			inputCode:    ErrImageStreamGetterForbiddenCode,
			expectedCode: ErrImageStreamForbiddenCode,
		},
		{
			name:         "Unknown error remains as ImageStream Unknown",
			inputCode:    ErrImageStreamGetterUnknownCode,
			expectedCode: ErrImageStreamUnknownErrorCode,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inputErr := rerrors.NewError(tc.inputCode, "test/repo", nil)
			result := convertImageStreamGetterError(inputErr, "test message")

			if result.Code() != tc.expectedCode {
				t.Errorf("expected error code %q, got %q", tc.expectedCode, result.Code())
			}

			if result.Message() != "test message" {
				t.Errorf("expected message %q, got %q", "test message", result.Message())
			}
		})
	}
}
