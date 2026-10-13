package metrics

import (
	"errors"
	"io/fs"
	"syscall"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
)

const (
	// errCodeUnsupportedMethod happens when a storage driver does not offer
	// support for a determine http method, or when it doesn't support url
	// redirects. this error usually bubbles up back to clients, making it
	// easy to detect. fixing it might simply involve disabling redirects
	// the image registry.
	errCodeUnsupportedMethod = "UNSUPPORTED_METHOD"

	// errCodePathNotFound indicates that a given path does not exist in
	// the underlying storage. it usually indicates a user error and should
	// not be considered high severity.
	errCodePathNotFound = "PATH_NOT_FOUND"

	// errCodeInvalidPath indicates the provided path does not comform to
	// the upstream distribution standards.
	// see https://github.com/distribution/distribution/blob/bce9fcd135940c4be187f6fc98c2e27dad9ddcea/registry/storage/driver/storagedriver.go?plain=1#L134-L139
	// for details.
	errCodeInvalidPath = "INVALID_PATH"

	// errCodeInvalidOffset indicates an attempt to read/write at an invalid
	// offset. A valid offset is contextual. Storage drivers usually specify
	// non-zero offsets when reading a stream of bytes from storage, or
	// during chunked blob uploads.
	errCodeInvalidOffset = "INVALID_OFFSET"

	// errCodeReadOnlyFS indicates the filesystem the registry is trying to
	// write is read-only. this is a catastrophic error and needs admin
	// intervention for recovery. however, clients will only notice if they
	// try to push directly into the registry. pull-through will transparently
	// fail and failures will only be visible in logs and metrics/alerts.
	errCodeReadOnlyFS = "READ_ONLY_FILESYSTEM"

	// errCodeFileTooLarge indicates the file being uploaded is too large.
	errCodeFileTooLarge = "FILE_TOO_LARGE"

	// errCodeDeviceOutOfSpace indicates the storage device is out of space.
	errCodeDeviceOutOfSpace = "DEVICE_OUT_OF_SPACE"

	errCodeUnknown = "UNKNOWN"

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
