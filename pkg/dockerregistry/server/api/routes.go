package api

import (
	"github.com/distribution/distribution/v3/reference"
)

var (
	AdminPrefix      = "/admin/"
	ExtensionsPrefix = "/extensions/v2/"

	AdminPath      = "/blobs/{digest:" + reference.DigestRegexp.String() + "}"
	SignaturesPath = "/{name:" + reference.NameRegexp.String() + "}/signatures/{digest:" + reference.DigestRegexp.String() + "}"
	MetricsPath    = "/metrics"
)
