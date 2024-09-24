package metrics

import storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"

const (
	errCodeUnsupportedMethod = "UNSUPPORTED_METHOD"
	errCodePathNotFound      = "PATH_NOT_FOUND"
	errCodeInvalidPath       = "INVALID_PATH"
	errCodeInvalidOffset     = "INVALID_OFFSET"
	errCodeUnknown           = "UNKNOWN"
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
	}

	return errCodeUnknown
}
