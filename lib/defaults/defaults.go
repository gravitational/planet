package defaults

const (
	// ContainerProcessLabel specifies the default SELinux label for processes inside the container
	ContainerProcessLabel = "system_u:system_r:gravity_container_t:s0"

	// ContainerFileLabel specifies the default SELinux label for files inside the container
	ContainerFileLabel = "system_u:object_r:container_file_t:s0"

	// PlanetDataDir specifies the location for libcontainer-specific data
	PlanetDataDir = "/var/run/planet"

	// InitUser specifies the user for the init process
	InitUser = "root"

	// ContainerBaseUID specifies the initial user ID for the host-container
	// mapping
	ContainerBaseUID = 1000
	// ContainerBaseGID specifies the initial groupID for the host-container
	// mapping
	ContainerBaseGID = 1000
)

var (
	// InitArgs specifies the command line for the init process
	InitArgs = []string{"/bin/systemd"}
)
