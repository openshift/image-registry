package testutil

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/reference"
	distclient "github.com/docker/distribution/registry/client"
	"github.com/docker/distribution/registry/client/auth"
	"github.com/docker/distribution/registry/client/auth/challenge"
	"github.com/docker/distribution/registry/client/transport"
	"github.com/opencontainers/go-digest"

	"k8s.io/apimachinery/pkg/util/errors"

	imageapiv1 "github.com/openshift/api/image/v1"
	imageapi "github.com/openshift/image-registry/pkg/origin-common/image/apis/image"
)

func NewTransport(baseURL string, repoName string, creds auth.CredentialStore) (http.RoundTripper, error) {
	httpTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	challengeManager := challenge.NewSimpleManager()

	_, err := ping(httpTransport, challengeManager, baseURL+"/v2/", "")
	if err != nil {
		return nil, err
	}

	if creds == nil {
		return httpTransport, nil
	}

	return transport.NewTransport(
		httpTransport,
		auth.NewAuthorizer(
			challengeManager,
			auth.NewTokenHandler(nil, creds, repoName, "pull", "push"),
			auth.NewBasicHandler(creds),
		),
	), nil
}

// NewRepository creates a new Repository for the given repository name, base URL and creds.
func NewRepository(repoName string, baseURL string, transport http.RoundTripper) (distribution.Repository, error) {
	ref, err := reference.WithName(repoName)
	if err != nil {
		return nil, err
	}

	return distclient.NewRepository(ref, baseURL, transport)
}

func NewInsecureRepository(imageReference string, creds auth.CredentialStore) (distribution.Repository, error) {
	ref, err := reference.ParseNamed(imageReference)
	if err != nil {
		return nil, fmt.Errorf("unable to parse image reference %s: %w", imageReference, err)
	}

	pathRef, err := reference.WithName(reference.Path(ref))
	if err != nil {
		return nil, fmt.Errorf("unable to get path reference for %s: %w", imageReference, err)
	}

	var baseURL string
	var transport http.RoundTripper
	var errs []error
	found := false
	for _, scheme := range []string{"https", "http"} {
		baseURL = scheme + "://" + reference.Domain(ref)

		transport, err = NewTransport(baseURL, reference.Path(ref), creds)
		if err != nil {
			errs = append(errs, fmt.Errorf("unable to get %s transport for %s: %w", scheme, imageReference, err))
			continue
		}

		found = true
		break
	}
	if !found {
		return nil, errors.NewAggregate(errs)
	}

	return distclient.NewRepository(pathRef, baseURL, transport)
}

// UploadBlob uploads the blob with content to repo and verifies its digest.
func UploadBlob(ctx context.Context, repo distribution.Repository, desc distribution.Descriptor, content []byte) error {
	wr, err := repo.Blobs(ctx).Create(ctx)
	if err != nil {
		return fmt.Errorf("failed to create upload to %s: %w", repo.Named(), err)
	}

	_, err = io.Copy(wr, bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("error uploading blob to %s: %w", repo.Named(), err)
	}

	uploadDesc, err := wr.Commit(ctx, distribution.Descriptor{
		Digest: digest.FromBytes(content),
	})
	if err != nil {
		return fmt.Errorf("failed to complete upload to %s: %w", repo.Named(), err)
	}

	// uploadDesc is checked here and is not returned, because it has invalid MediaType.
	if uploadDesc.Digest != desc.Digest {
		return fmt.Errorf("upload blob to %s failed: digest mismatch: got %s, want %s", repo.Named(), uploadDesc.Digest, desc.Digest)
	}

	return nil
}

// UploadManifest uploads manifest to repo and verifies its digest.
func UploadManifest(ctx context.Context, repo distribution.Repository, tag string, manifest distribution.Manifest) error {
	canonical, err := CanonicalManifest(manifest)
	if err != nil {
		return err
	}

	ms, err := repo.Manifests(ctx)
	if err != nil {
		return fmt.Errorf("failed to get manifest service for %s: %w", repo.Named(), err)
	}

	dgst, err := ms.Put(ctx, manifest, distribution.WithTag(tag))
	if err != nil {
		return fmt.Errorf("failed to upload manifest to %s: %w", repo.Named(), err)
	}

	if expectedDgst := digest.FromBytes(canonical); dgst != expectedDgst {
		return fmt.Errorf("upload manifest to %s failed: digest mismatch: got %s, want %s", repo.Named(), dgst, expectedDgst)
	}

	return nil
}

// UploadRandomTestBlob generates a random tar file and uploads it to the given repository.
func UploadRandomTestBlob(ctx context.Context, baseURL string, creds auth.CredentialStore, repoName string) (distribution.Descriptor, []byte, error) {
	payload, desc, err := MakeRandomLayer()
	if err != nil {
		return distribution.Descriptor{}, nil, fmt.Errorf("unexpected error generating test layer file: %w", err)
	}

	rt, err := NewTransport(baseURL, repoName, creds)
	if err != nil {
		return distribution.Descriptor{}, nil, err
	}

	repo, err := NewRepository(repoName, baseURL, rt)
	if err != nil {
		return distribution.Descriptor{}, nil, err
	}

	err = UploadBlob(ctx, repo, desc, payload)
	if err != nil {
		return distribution.Descriptor{}, nil, fmt.Errorf("upload random test blob: %w", err)
	}

	return desc, payload, nil
}

// CreateRandomTarFile creates a random tarfile and returns its content.
// An error is returned if there is a problem generating valid content.
// Inspired by github.com/docker/distribution/testutil/tarfile.go.
func CreateRandomTarFile() ([]byte, error) {
	nFiles := 2 // random enough

	var target bytes.Buffer
	wr := tar.NewWriter(&target)

	// Perturb this on each iteration of the loop below.
	header := &tar.Header{
		Mode:       0644,
		ModTime:    time.Now(),
		Typeflag:   tar.TypeReg,
		Uname:      "randocalrissian",
		Gname:      "cloudcity",
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
	}

	for fileNumber := 0; fileNumber < nFiles; fileNumber++ {
		header.Name = fmt.Sprint(fileNumber)
		header.Size = mrand.Int63n(1<<9) + 1<<9

		err := wr.WriteHeader(header)
		if err != nil {
			return nil, err
		}

		randomData := make([]byte, header.Size)
		_, err = rand.Read(randomData)
		if err != nil {
			return nil, err
		}

		_, err = io.Copy(wr, bytes.NewReader(randomData))
		if err != nil {
			return nil, err
		}
	}

	if err := wr.Close(); err != nil {
		return nil, err
	}

	return target.Bytes(), nil
}

// CreateRandomImage creates an image with a random content.
func CreateRandomImage(namespace, name string) (*imageapiv1.Image, error) {
	const layersCount = 2

	layersDescs := make([]distribution.Descriptor, layersCount)
	for i := range layersDescs {
		_, desc, err := MakeRandomLayer()
		if err != nil {
			return nil, err
		}
		layersDescs[i] = desc
	}

	manifest, err := MakeSchema1Manifest("unused-name", "unused-tag", layersDescs)
	if err != nil {
		return nil, err
	}

	_, manifestSchema1, err := manifest.Payload()
	if err != nil {
		return nil, err
	}

	return NewImageForManifest(
		fmt.Sprintf("%s/%s", namespace, name),
		string(manifestSchema1),
		"",
		false,
	)
}

const SampleImageManifestSchema1 = `{
   "schemaVersion": 1,
   "name": "nm/is",
   "tag": "latest",
   "architecture": "",
   "fsLayers": [
      {
         "blobSum": "sha256:b2c5513bd934a7efb412c0dd965600b8cb00575b585eaff1cb980b69037fe6cd"
      },
      {
         "blobSum": "sha256:2dde6f11a89463bf20dba3b47d8b3b6de7cdcc19e50634e95a18dd95c278768d"
      }
   ],
   "history": [
      {
         "v1Compatibility": "{\"size\":18407936}"
      },
      {
         "v1Compatibility": "{\"size\":19387392}"
      }
   ],
   "signatures": [
      {
         "header": {
            "jwk": {
               "crv": "P-256",
               "kid": "5HTY:A24B:L6PG:TQ3G:GMAK:QGKZ:ICD4:S7ZJ:P5JX:UTMP:XZLK:ZXVH",
               "kty": "EC",
               "x": "j5YnDSyrVIt3NquUKvcZIpbfeD8HLZ7BVBFL4WutRBM",
               "y": "PBgFAZ3nNakYN3H9enhrdUrQ_HPYzb8oX5rtJxJo1Y8"
            },
            "alg": "ES256"
         },
         "signature": "1rXiEmWnf9eL7m7Wy3K4l25-Zv2XXl5GgqhM_yjT0ujPmTn0uwfHcCWlweHa9gput3sECj507eQyGpBOF5rD6Q",
         "protected": "eyJmb3JtYXRMZW5ndGgiOjQ4NSwiZm9ybWF0VGFpbCI6IkNuMCIsInRpbWUiOiIyMDE2LTA3LTI2VDExOjQ2OjQ2WiJ9"
      }
   ]
}`

type testCredentialStore struct {
	username      string
	password      string
	refreshTokens map[string]string
}

var _ auth.CredentialStore = &testCredentialStore{}

// NewBasicCredentialStore returns a test credential store for use with registry token handler and/or basic
// handler.
func NewBasicCredentialStore(username, password string) auth.CredentialStore {
	return &testCredentialStore{
		username: username,
		password: password,
	}
}

func (tcs *testCredentialStore) Basic(*url.URL) (string, string) {
	return tcs.username, tcs.password
}

func (tcs *testCredentialStore) RefreshToken(u *url.URL, service string) string {
	return tcs.refreshTokens[service]
}

func (tcs *testCredentialStore) SetRefreshToken(u *url.URL, service string, token string) {
	if tcs.refreshTokens != nil {
		tcs.refreshTokens[service] = token
	}
}

// ping pings the provided endpoint to determine its required authorization challenges.
// If a version header is provided, the versions will be returned.
func ping(transport http.RoundTripper, manager challenge.Manager, endpoint, versionHeader string) ([]auth.APIVersion, error) {
	client := &http.Client{
		Transport: transport,
	}

	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer func() {
		// TODO(dmage): log error?
		_ = resp.Body.Close()
	}()

	if err := manager.AddResponse(resp); err != nil {
		return nil, err
	}

	versions := auth.APIVersions(resp, versionHeader)
	if len(versions) == 0 {
		ok := resp.StatusCode >= 200 && resp.StatusCode < 300 ||
			resp.StatusCode == http.StatusUnauthorized ||
			resp.StatusCode == http.StatusForbidden
		if !ok {
			return nil, fmt.Errorf("registry does not support v2 API: got %s from %s", resp.Status, endpoint)
		}
	}

	return versions, nil
}

// UploadSchema2Image creates a random image with a schema 2 manifest and
// uploads it to the repository.
func UploadSchema2Image(ctx context.Context, repo distribution.Repository, tag string) (distribution.Manifest, error) {
	const layersCount = 2

	layers := make([]distribution.Descriptor, layersCount)
	for i := range layers {
		content, desc, err := MakeRandomLayer()
		if err != nil {
			return nil, fmt.Errorf("make random layer: %w", err)
		}

		if err := UploadBlob(ctx, repo, desc, content); err != nil {
			return nil, fmt.Errorf("upload random blob: %w", err)
		}

		layers[i] = desc
	}

	cfg := map[string]interface{}{
		"rootfs": map[string]interface{}{
			"diff_ids": make([]string, len(layers)),
		},
		"history": make([]struct{}, len(layers)),
	}

	configContent, err := json.Marshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal image config: %w", err)
	}

	config := distribution.Descriptor{
		Digest: digest.FromBytes(configContent),
		Size:   int64(len(configContent)),
	}

	if err := UploadBlob(ctx, repo, config, configContent); err != nil {
		return nil, fmt.Errorf("upload image config: %w", err)
	}

	manifest, err := MakeSchema2Manifest(config, layers)
	if err != nil {
		return manifest, fmt.Errorf("make schema 2 manifest: %w", err)
	}

	if err := UploadManifest(ctx, repo, tag, manifest); err != nil {
		return manifest, fmt.Errorf("upload schema 2 manifest: %w", err)
	}

	return manifest, nil
}

func ConvertImage(image *imageapi.Image) (*imageapiv1.Image, error) {
	newImage := &imageapiv1.Image{}
	newImage.Name = image.Name
	newImage.Annotations = image.Annotations
	newImage.DockerImageReference = image.DockerImageReference
	newImage.DockerImageManifest = image.DockerImageManifest
	newImage.DockerImageConfig = image.DockerImageConfig

	for _, layer := range image.DockerImageLayers {
		newImage.DockerImageLayers = append(newImage.DockerImageLayers, imageapiv1.ImageLayer{
			Name:      layer.Name,
			LayerSize: layer.LayerSize,
			MediaType: layer.MediaType,
		})
	}
	b, err := json.Marshal(image.DockerImageMetadata)
	if err != nil {
		return nil, err
	}
	newImage.DockerImageMetadata.Raw = b
	return newImage, nil
}

func VerifyRemoteImage(ctx context.Context, repo distribution.Repository, tag string) (mediatype string, dgst digest.Digest, err error) {
	ms, err := repo.Manifests(ctx)
	if err != nil {
		return "", "", fmt.Errorf("verify %s:%s: get manifest service: %w", repo.Named(), tag, err)
	}

	m, err := ms.Get(ctx, "", distribution.WithTag(tag))
	if err != nil {
		return "", "", fmt.Errorf("verify %s:%s: get manifest: %w", repo.Named(), tag, err)
	}

	mediatype, payload, err := m.Payload()
	if err != nil {
		return mediatype, "", fmt.Errorf("verify %s:%s: get manifest payload: %w", repo.Named(), tag, err)
	}

	dgst = digest.FromBytes(payload)

	bs := repo.Blobs(ctx)
	for _, desc := range m.References() {
		r, err := bs.Open(ctx, desc.Digest)
		if err != nil {
			return mediatype, dgst, fmt.Errorf("verify %s:%s: open blob %s: %w", repo.Named(), tag, desc.Digest, err)
		}
		dgst, readErr := digest.FromReader(r)
		closeErr := r.Close()
		if readErr != nil {
			return mediatype, dgst, fmt.Errorf("verify %s:%s: read blob %s: %w", repo.Named(), tag, desc.Digest, readErr)
		}
		if closeErr != nil {
			return mediatype, dgst, fmt.Errorf("verify %s:%s: close blob %s: %w", repo.Named(), tag, desc.Digest, closeErr)
		}
		if dgst != desc.Digest {
			return mediatype, dgst, fmt.Errorf("verify %s:%s: blob digest mismatch: got %q, want %q", repo.Named(), tag, dgst, desc.Digest)
		}
	}

	return mediatype, dgst, nil
}
