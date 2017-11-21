package crypto

import (
	"crypto/tls"
	"fmt"
	"sort"
)

var versions = map[string]uint16{
	"VersionTLS10": tls.VersionTLS10,
	"VersionTLS11": tls.VersionTLS11,
	"VersionTLS12": tls.VersionTLS12,
}

func TLSVersion(versionName string) (uint16, error) {
	if len(versionName) == 0 {
		return DefaultTLSVersion(), nil
	}
	if version, ok := versions[versionName]; ok {
		return version, nil
	}
	return 0, fmt.Errorf("unknown tls version %q", versionName)
}

func ValidTLSVersions() []string {
	validVersions := []string{}
	for k := range versions {
		validVersions = append(validVersions, k)
	}
	sort.Strings(validVersions)
	return validVersions
}

func DefaultTLSVersion() uint16 {
	// Can't use SSLv3 because of POODLE and BEAST
	// Can't use TLSv1.0 because of POODLE and BEAST using CBC cipher
	// Can't use TLSv1.1 because of RC4 cipher usage
	return tls.VersionTLS12
}

var ciphers = map[string]uint16{
	"TLS_RSA_WITH_RC4_128_SHA":                tls.TLS_RSA_WITH_RC4_128_SHA,
	"TLS_RSA_WITH_3DES_EDE_CBC_SHA":           tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	"TLS_RSA_WITH_AES_128_CBC_SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"TLS_RSA_WITH_AES_256_CBC_SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	"TLS_RSA_WITH_AES_128_CBC_SHA256":         tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
	"TLS_RSA_WITH_AES_128_GCM_SHA256":         tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_RSA_WITH_AES_256_GCM_SHA384":         tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_ECDSA_WITH_RC4_128_SHA":        tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_RC4_128_SHA":          tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
	"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA":     tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
	"TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":   tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384": tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305":    tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305":  tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
}

func CipherSuite(cipherName string) (uint16, error) {
	if cipher, ok := ciphers[cipherName]; ok {
		return cipher, nil
	}
	return 0, fmt.Errorf("unknown cipher name %q", cipherName)
}

func ValidCipherSuites() []string {
	validCipherSuites := []string{}
	for k := range ciphers {
		validCipherSuites = append(validCipherSuites, k)
	}
	sort.Strings(validCipherSuites)
	return validCipherSuites
}

func DefaultCiphers() []uint16 {
	// HTTP/2 mandates TLS 1.2 or higher with an AEAD cipher
	// suite (GCM, Poly1305) and ephemeral key exchange (ECDHE, DHE) for
	// perfect forward secrecy. Servers may provide additional cipher
	// suites for backwards compatibility with HTTP/1.1 clients.
	// See RFC7540, section 9.2 (Use of TLS Features) and Appendix A
	// (TLS 1.2 Cipher Suite Black List).
	return []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, // required by http/2
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256, // forbidden by http/2, not flagged by http2isBadCipher() in go1.8
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,   // forbidden by http/2, not flagged by http2isBadCipher() in go1.8
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,    // forbidden by http/2
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,    // forbidden by http/2
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,      // forbidden by http/2
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,      // forbidden by http/2
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,         // forbidden by http/2
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,         // forbidden by http/2
		// the next one is in the intermediate suite, but go1.8 http2isBadCipher() complains when it is included at the recommended index
		// because it comes after ciphers forbidden by the http/2 spec
		// tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		// tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA, // forbidden by http/2, disabled to mitigate SWEET32 attack
		// tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,       // forbidden by http/2, disabled to mitigate SWEET32 attack
		tls.TLS_RSA_WITH_AES_128_CBC_SHA, // forbidden by http/2
		tls.TLS_RSA_WITH_AES_256_CBC_SHA, // forbidden by http/2
	}
}

// SecureTLSConfig enforces the default minimum security settings for the cluster.
func SecureTLSConfig(config *tls.Config) *tls.Config {
	if config.MinVersion == 0 {
		config.MinVersion = DefaultTLSVersion()
	}

	config.PreferServerCipherSuites = true
	if len(config.CipherSuites) == 0 {
		config.CipherSuites = DefaultCiphers()
	}
	return config
}
