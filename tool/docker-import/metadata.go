package main

import (
	"fmt"
	"path/filepath"
)

// Repositories describes docker container image repository metadata.
type Repositories map[string]Image

// Image describes a container image metadata.
type Image map[string]string

// Images returns the list of container images this repositories value
// contains
func (r Repositories) Images() (result []image) {
	result = make([]image, 0, len(r))
	for path, metadata := range r {
		result = append(result, image{
			path:     path,
			metadata: metadata,
		})
	}
	return result
}

// url builds the container image repository URL:
//
// repository/name:version
func (r image) url() string {
	repoURL, imageName := filepath.Split(r.path)
	for version := range r.metadata {
		return filepath.Join(repoURL, imageTag(imageName, version))
	}
	return ""
}

type image struct {
	// path specifies the complete image path
	path     string
	metadata map[string]string
}

func imageTag(name, version string) string {
	return fmt.Sprintf("%v:%v", name, version)
}
