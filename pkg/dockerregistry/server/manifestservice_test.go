package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/manifestlist"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/opencontainers/go-digest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageapiv1 "github.com/openshift/api/image/v1"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
	registryclient "github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/metrics"
	"github.com/openshift/image-registry/pkg/imagestream"
	"github.com/openshift/image-registry/pkg/testutil"
)

func TestManifestServiceExists(t *testing.T) {
	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)

	namespace := "user"
	repo := "app"
	tag := "latest"

	fos, imageClient := testutil.NewFakeOpenShiftWithClient(ctx)
	testImage := testutil.AddRandomImage(t, fos, namespace, repo, tag)

	client := registryclient.NewFakeRegistryAPIClient(nil, imageClient)
	imageStream := imagestream.New(ctx, namespace, repo, client)

	ms := &manifestService{
		imageStream:      imageStream,
		registryOSClient: client,
		acceptSchema2:    true,
	}

	ok, err := ms.Exists(ctx, digest.Digest(testImage.Name))
	if err != nil {
		t.Errorf("ms.Exists(ctx, %q): %s", testImage.Name, err)
	} else if !ok {
		t.Errorf("ms.Exists(ctx, %q): got false, want true", testImage.Name)
	}

	_, err = ms.Exists(ctx, unknownBlobDigest)
	if err == nil {
		t.Errorf("ms.Exists(ctx, %q): got success, want error", unknownBlobDigest)
	}
}

func TestManifestServiceGet(t *testing.T) {
	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)

	testCases := []struct {
		name      string
		namespace string
		repo      string
		tag       string
		input     digest.Digest
		image     *imageapiv1.Image
		manifest  distribution.Manifest
	}{
		{
			name:      "single image manifest",
			namespace: "user",
			repo:      "app",
			tag:       "latest",
			input:     digest.Digest("sha256:abd132"),
			image: &imageapiv1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sha256:abd132",
					Namespace: "user",
				},
				DockerImageReference: "quay.io/openshift/test-image@sha256:abd132",
			},
			manifest: &schema2.DeserializedManifest{
				Manifest: schema2.Manifest{
					Config: distribution.Descriptor{
						Digest: "sha256:a1b2d",
						Size:   2,
					},
				},
			},
		},
		{
			name:      "manifest list",
			namespace: "user",
			repo:      "app",
			tag:       "latest",
			input:     digest.Digest("sha256:afd142"),
			image: &imageapiv1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sha256:afd142",
					Namespace: "user",
				},
				DockerImageReference: "quay.io/openshift/test-multi-arch-image@sha256:afd142",
				DockerImageManifests: []imageapiv1.ImageManifest{
					{
						Digest:       "sha256:fde333",
						Architecture: "amd64",
						OS:           "linux",
					},
				},
			},
			manifest: &manifestlist.DeserializedManifestList{
				ManifestList: manifestlist.ManifestList{
					Manifests: []manifestlist.ManifestDescriptor{
						{
							Descriptor: distribution.Descriptor{
								Digest: "sha256:fde333",
							},
						},
					},
				},
			},
		},
		{
			name:      "sub manifest of manifest list",
			namespace: "user",
			repo:      "app",
			input:     digest.Digest("sha256:fde333"),
			image: &imageapiv1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sha256:c343983d08baf2b3cc19483734808b07523a0860a5b17fdcb32de2d1b85306ca",
					Namespace: "user",
				},
				DockerImageReference: "quay.io/openshift/test-multi-arch@sha256:c343983d08baf2b3cc19483734808b07523a0860a5b17fdcb32de2d1b85306ca",
				DockerImageManifests: []imageapiv1.ImageManifest{
					{
						Digest:       "sha256:fde333",
						Architecture: "amd64",
						OS:           "linux",
					},
				},
			},
			manifest: &manifestlist.DeserializedManifestList{
				ManifestList: manifestlist.ManifestList{
					Manifests: []manifestlist.ManifestDescriptor{
						{
							Descriptor: distribution.Descriptor{
								Digest: "sha256:fde333",
							},
						},
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fos, imageClient := testutil.NewFakeOpenShiftWithClient(ctx)

			client := registryclient.NewFakeRegistryAPIClient(nil, imageClient)
			imageStream := imagestream.New(ctx, testCase.namespace, testCase.repo, client)

			repoName := fmt.Sprintf("%s/%s", testCase.namespace, testCase.repo)

			digestCache, err := cache.NewBlobDigest(
				defaultDescriptorCacheSize,
				defaultDigestToRepositoryCacheSize,
				24*time.Hour, // for tests it's virtually forever
				metrics.NewNoopMetrics(),
			)
			if err != nil {
				t.Fatalf("unable to create cache: %v", err)
			}
			cache := cache.NewRepositoryDigest(digestCache)

			localManifestData := map[digest.Digest]distribution.Manifest{
				digest.Digest(testCase.image.Name): testCase.manifest,
			}

			testutil.AddImageStream(t, fos, testCase.namespace, testCase.repo, nil)
			testutil.AddImage(t, fos, testCase.image, testCase.namespace, testCase.repo, testCase.tag)

			// create any sub-images and sub-manifests when testCase.Image refers to a
			// manifest list. it's important here that sub-images are "untagged", which
			// means that they are not directly associated to an image stream, because
			// that's how production code works.
			if len(testCase.image.DockerImageManifests) > 0 {
				for _, subImage := range testCase.image.DockerImageManifests {
					testutil.AddUntaggedImage(t, fos, &imageapiv1.Image{
						ObjectMeta: metav1.ObjectMeta{
							Name: subImage.Digest,
						},
					})

					manifest, err := schema2.FromStruct(schema2.Manifest{
						Config: distribution.Descriptor{
							Digest: "sha256:cf123",
						},
					})
					if err != nil {
						t.Fatalf("failed to intialise deserialised manifest: %s", err)
					}
					localManifestData[digest.Digest(subImage.Digest)] = manifest
				}
			}

			ms := &manifestService{
				cache:            cache,
				manifests:        newTestManifestService(repoName, localManifestData),
				imageStream:      imageStream,
				registryOSClient: client,
				acceptSchema2:    true,
			}

			manifest, err := ms.Get(ctx, testCase.input)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			_ = manifest
			// TODO: assert that a manifest list's image has all the necessary
			// fields correctly filled up
		})
	}
}

func TestManifestServicePut(t *testing.T) {
	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)

	namespace := "user"
	repo := "app"
	repoName := fmt.Sprintf("%s/%s", namespace, repo)

	testCases := []struct {
		name              string
		manifestType      string
		parentManifest    bool
		createImageStream bool
		tag               string
	}{
		{
			name:         "ByTag",
			manifestType: schema2.MediaTypeManifest,
			tag:          "latest",
		},
		{
			name:              "ByDigest",
			manifestType:      schema2.MediaTypeManifest,
			parentManifest:    true,
			createImageStream: true,
		},
		{
			name:         "ManifestList",
			manifestType: manifestlist.MediaTypeManifestList,
			tag:          "latest",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fos, imageClient := testutil.NewFakeOpenShiftWithClient(ctx)
			bs := newTestBlobStore(nil, blobContents{
				"testblob:1":   []byte("{}"),
				"testconfig:2": []byte("{}"),
			})
			tms := newTestManifestService(repoName, nil)
			client := registryclient.NewFakeRegistryAPIClient(nil, imageClient)
			imageStream := imagestream.New(ctx, namespace, repo, client)
			digestCache, err := cache.NewBlobDigest(
				defaultDescriptorCacheSize,
				defaultDigestToRepositoryCacheSize,
				24*time.Hour, // for tests it's virtually forever
				metrics.NewNoopMetrics(),
			)
			if err != nil {
				t.Fatalf("unable to create cache: %v", err)
			}
			ms := &manifestService{
				serverAddr:       "localhost",
				manifests:        tms,
				blobStore:        bs,
				registryOSClient: client,
				imageStream:      imageStream,
				acceptSchema2:    true,
			}
			osclient, err := registryclient.NewFakeRegistryClient(imageClient).Client()
			if err != nil {
				t.Fatal(err)
			}

			if testCase.createImageStream {
				testutil.AddImageStream(t, fos, namespace, repo, nil)
			}

			putCtx := withAuthPerformed(ctx)
			putCtx = withUserClient(putCtx, osclient)
			var options []distribution.ManifestServiceOption
			if len(testCase.tag) > 0 {
				options = []distribution.ManifestServiceOption{
					distribution.WithTag(testCase.tag),
				}
			}
			var manifest distribution.Manifest
			switch testCase.manifestType {
			case schema2.MediaTypeManifest:
				var err error
				manifest, err = testutil.MakeSchema2Manifest(
					distribution.Descriptor{
						Digest: "testconfig:2",
						Size:   2,
					},
					[]distribution.Descriptor{
						{
							Digest: "testblob:1",
							Size:   2,
						},
					},
				)
				if err != nil {
					t.Fatalf("could not make schema 2 manifest: %s", err)
				}
			case manifestlist.MediaTypeManifestList:
				var err error
				manifest, err = manifestlist.FromDescriptors([]manifestlist.ManifestDescriptor{
					{
						Descriptor: distribution.Descriptor{
							MediaType: schema2.MediaTypeManifest,
							Digest:    "sha256:240a11",
							Size:      529,
						},
						Platform: manifestlist.PlatformSpec{
							Architecture: "amd64",
							OS:           "linux",
						},
					},
				})
				if err != nil {
					t.Fatalf("could not make manifest list: %s", err)
				}
			default:
				t.Fatalf("this test has not yet learned how to handle media type %q", testCase.manifestType)
			}

			dgst, err := ms.Put(putCtx, manifest, options...)
			if err != nil {
				t.Fatalf("failed to Put manifest: %s", err)
			}

			// create the parent manifest if the test case gave us one.
			// a parent manifest is needed when pulling a sub-manifest by digest.
			// if no parent manifest exists, the sub-manifest has no link to the image
			// stream, and without that link the registry will not be able to retrieve it.
			if testCase.parentManifest {
				parentManifest, err := manifestlist.FromDescriptors([]manifestlist.ManifestDescriptor{
					{
						Descriptor: distribution.Descriptor{
							MediaType: schema2.MediaTypeManifest,
							Digest:    dgst, // use sub-manifest digest
							Size:      529,
						},
						Platform: manifestlist.PlatformSpec{
							Architecture: "amd64",
							OS:           "linux",
						},
					},
				})
				if err != nil {
					t.Fatalf("unable to instatiate deserialized manifest list struct: %s", err)
				}

				opts := []distribution.ManifestServiceOption{distribution.WithTag("latest")}
				_, err = ms.Put(putCtx, parentManifest, opts...)
				if err != nil {
					t.Fatalf("failed to Put parent manifest: %s", err)
				}
			}

			// recreate objects to reset cached image streams
			imageStream = imagestream.New(ctx, namespace, repo, registryclient.NewFakeRegistryAPIClient(nil, imageClient))

			ms = &manifestService{
				manifests:        tms,
				registryOSClient: client,
				imageStream:      imageStream,
				cache:            cache.NewRepositoryDigest(digestCache),
				acceptSchema2:    true,
			}

			// TODO: add some assertions
			_, err = ms.Get(ctx, dgst)
			if err != nil {
				t.Errorf("failed to get manifest with digest %q: %s", dgst, err)
			}
		})
	}
}
