package image

import (
	"fmt"
	"strings"

	"github.com/distribution/distribution/v3/reference"
)

// NamedDockerImageReference points to a Docker image.
type namedDockerImageReference struct {
	Registry  string
	Namespace string
	Name      string
	Tag       string
	ID        string
}

// parseNamedDockerImageReference parses a Docker pull spec string into a
// NamedDockerImageReference.
func parseNamedDockerImageReference(spec string) (namedDockerImageReference, error) {
	var ref namedDockerImageReference

	parsedRef, err := reference.Parse(spec)
	if err != nil {
		return ref, err
	}

	namedRef, isNamed := parsedRef.(reference.Named)
	if !isNamed {
		return ref, fmt.Errorf("reference %s has no name", parsedRef.String())
	}

	name := namedRef.Name()
	i := strings.IndexRune(name, '/')
	if i == -1 || (!strings.ContainsAny(name[:i], ":.") && name[:i] != "localhost") {
		ref.Name = name
	} else {
		ref.Registry, ref.Name = name[:i], name[i+1:]
	}

	if named, ok := namedRef.(reference.NamedTagged); ok {
		ref.Tag = named.Tag()
	}

	if named, ok := namedRef.(reference.Canonical); ok {
		ref.ID = named.Digest().String()
	}

	// It's not enough just to use the reference.ParseNamed(). We have to fill
	// ref.Namespace from ref.Name
	if i := strings.IndexRune(ref.Name, '/'); i != -1 {
		ref.Namespace, ref.Name = ref.Name[:i], ref.Name[i+1:]
	}

	return ref, nil
}
