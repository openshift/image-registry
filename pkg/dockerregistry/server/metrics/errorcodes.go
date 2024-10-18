package metrics

import (
	"errors"
	"io/fs"
	"syscall"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
)

const (
	errCodeUnsupportedMethod = "UNSUPPORTED_METHOD"
	errCodePathNotFound      = "PATH_NOT_FOUND"
	errCodeInvalidPath       = "INVALID_PATH"
	errCodeInvalidOffset     = "INVALID_OFFSET"
	errCodeReadOnlyFS        = "READ_ONLY_FILESYSTEM"
	errCodeFileTooLarge      = "FILE_TOO_LARGE"
	errCodeDeviceOutOfSpace  = "DEVICE_OUT_OF_SPACE"
	errCodeUnknown           = "UNKNOWN"

	// errnoEFBIG represents a file too large error.
	// See `man errno` for more.
	errnoEFBIG = 27
	// errnoENOSPC represents a device out of space error.
	// See `man errno` for more.
	errnoENOSPC = 28
	// errnoEROFS represents the syscall error number returned when an attempt
	// to write to a read-only filesystem was made.
	// See `man errno` for more.
	errnoEROFS = 30
)

func storageErrorCode(err error) string {
	switch actual := err.(type) {
	case storagedriver.ErrUnsupportedMethod:
		return errCodeUnsupportedMethod
	case storagedriver.PathNotFoundError:
		return errCodePathNotFound
	case storagedriver.InvalidPathError:
		return errCodeInvalidPath
	case storagedriver.InvalidOffsetError:
		return errCodeInvalidOffset
	case storagedriver.Error:
		var perr *fs.PathError
		if !errors.As(actual.Enclosed, &perr) {
			return errCodeUnknown
		}
		unwrapped := perr.Unwrap()
		if unwrapped == nil {
			return errCodeUnknown
		}
		errno, ok := unwrapped.(syscall.Errno)
		if !ok {
			return errCodeUnknown
		}
		return syscallErrnoToErrorCode(errno)

	}

	return errCodeUnknown
}

// syscallErrnoToErrorCode transforms the errno in an unwrapped fs.PathErr into
// a storage error code used to report metrics.
//
// if the given errno is not known to us, return errCodeUnknown.
func syscallErrnoToErrorCode(errno syscall.Errno) string {
	switch errno {
	case errnoEROFS:
		return errCodeReadOnlyFS
	case errnoEFBIG:
		return errCodeFileTooLarge
	case errnoENOSPC:
		return errCodeDeviceOutOfSpace
	}
	return errCodeUnknown
}
