// package check abstracts a process of running a health check.
package check

type Checker interface {
	// TODO: add context?
	// Check runs a health check and records any errors into the specified reporter.
	Check(Reporter)
}

type Reporter interface {
	// Adds a problem report identified by a name with the specified payload.
	Add(name string, payload []byte)
}
