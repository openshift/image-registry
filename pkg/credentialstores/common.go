package credentialstores

import (
	"net/url"
	"strings"

	"github.com/openshift/image-registry/pkg/kubernetes-common/credentialprovider"

	"github.com/golang/glog"
)

// MultiError is a wrap for multiple errors.
type MultiError struct {
	errors []string
}

// Append adds a new error.
func (m *MultiError) Append(e error) {
	m.errors = append(m.errors, e.Error())
}

// Error exists to comply with golang's error interface.
func (m *MultiError) Error() string {
	return strings.Join(m.errors, " ")
}

// basicCredentialsFromKeyring extract basicAuth information from provided
// keyring. If keyring does not contain information for the provided URL, empty
// strings are returned instead.
func basicCredentialsFromKeyring(keyring credentialprovider.DockerKeyring, target *url.URL) (string, string) {
	regURL := getURLForLookup(target)
	if configs, found := keyring.Lookup(regURL); found {
		glog.V(5).Infof(
			"Found secret to match %s (%s): %s",
			target, regURL, configs[0].ServerAddress,
		)
		return configs[0].Username, configs[0].Password
	}

	// do a special case check for docker.io to match historical lookups
	// when we respond to a challenge
	if regURL == "auth.docker.io/token" {
		glog.V(5).Infof(
			"Being asked for %s (%s), trying %s, legacy behavior",
			target, regURL, "index.docker.io/v1",
		)
		return basicCredentialsFromKeyring(
			keyring, &url.URL{Host: "index.docker.io", Path: "/v1"},
		)
	}

	// docker 1.9 saves 'docker.io' in config in f23, see
	// https://bugzilla.redhat.com/show_bug.cgi?id=1309739
	if regURL == "index.docker.io" {
		glog.V(5).Infof(
			"Being asked for %s (%s), trying %s, legacy behavior",
			target, regURL, "docker.io",
		)
		return basicCredentialsFromKeyring(
			keyring, &url.URL{Host: "docker.io"},
		)
	}

	// try removing the canonical ports.
	if hasCanonicalPort(target) {
		host := strings.SplitN(target.Host, ":", 2)[0]
		glog.V(5).Infof(
			"Being asked for %s (%s), trying %s without port",
			target, regURL, host,
		)
		return basicCredentialsFromKeyring(
			keyring,
			&url.URL{
				Scheme: target.Scheme,
				Host:   host,
				Path:   target.Path,
			},
		)
	}

	glog.V(5).Infof("Unable to find a secret to match %s (%s)",
		target, regURL,
	)
	return "", ""
}

// getURLForLookup returns the URL we should use when looking for credentials
// on a keyring.
func getURLForLookup(target *url.URL) string {
	var res string
	if target == nil {
		return res
	}

	if len(target.Scheme) == 0 || target.Scheme == "https" {
		res = target.Host + target.Path
	} else {
		// always require an explicit port to look up HTTP credentials
		if strings.Contains(target.Host, ":") {
			res = target.Host + target.Path
		} else {
			res = target.Host + ":80" + target.Path
		}
	}

	// Lookup(...) expects an image (not a URL path). The keyring strips /v1/ and /v2/
	// version prefixes so we should do the same when selecting a valid auth for a URL.
	pathWithSlash := target.Path + "/"
	if strings.HasPrefix(pathWithSlash, "/v1/") || strings.HasPrefix(pathWithSlash, "/v2/") {
		res = target.Host + target.Path[3:]
	}

	return res
}

// hasCanonicalPort returns if port is specified on the url and is the default
// port for the protocol.
func hasCanonicalPort(target *url.URL) bool {
	switch {
	case target == nil:
		return false
	case strings.HasSuffix(target.Host, ":443") && target.Scheme == "https":
		return true
	case strings.HasSuffix(target.Host, ":80") && target.Scheme == "http":
		return true
	default:
		return false
	}
}
