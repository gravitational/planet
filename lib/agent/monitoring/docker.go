package monitoring

import "io"

var dockerTags = Tags{
	"mode": {"node", "master"},
}

var dockerRegistryTags = Tags{
	"mode": {"master"},
}

func init() {
	AddChecker(newUnixSocketHealthzChecker("http://docker/version", "/var/run/docker.sock",
		dockerHealthz), "docker", dockerTags)
	AddChecker(newHTTPHealthzChecker("http://127.0.0.1:5000/v2/", dockerRegistryHealthz), "docker-registry", dockerRegistryTags)
}

func dockerHealthz(response io.Reader) error {
	// no-op
	return nil
}

func dockerRegistryHealthz(response io.Reader) error {
	// no-op
	return nil
}
