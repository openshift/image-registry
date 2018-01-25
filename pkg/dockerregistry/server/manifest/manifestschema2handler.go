package server

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema2"
)

func unmarshalManifestSchema2(content []byte) (distribution.Manifest, error) {
	var deserializedManifest schema2.DeserializedManifest
	if err := json.Unmarshal(content, &deserializedManifest); err != nil {
		return nil, err
	}

	if !reflect.DeepEqual(deserializedManifest.Versioned, schema2.SchemaVersion) {
		return nil, fmt.Errorf("unexpected manifest schema version=%d, mediaType=%q",
			deserializedManifest.SchemaVersion,
			deserializedManifest.MediaType)
	}

	return &deserializedManifest, nil
}
