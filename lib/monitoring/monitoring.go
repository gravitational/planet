package monitoring

import (
	"github.com/gravitational/planet/lib/agent/health"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

// defaultChecker is a health.Checker with a simplified interface.
type defaultChecker struct {
	name    string
	checker checker
}

type checker interface {
	check(reporter)
}

type reporter interface {
	add(error)
	addProbe(*pb.Probe)
}

func (r *defaultChecker) Name() string { return r.name }

// health.Checker
func (r *defaultChecker) Check(reporter health.Reporter) {
	rep := &delegatingReporter{Reporter: reporter, checker: r}
	r.checker.check(rep)
}

func newChecker(checker checker, name string) health.Checker {
	return &defaultChecker{name: name, checker: checker}
}

// delegatingReporter binds a checker to an external Reporter.
type delegatingReporter struct {
	health.Reporter
	checker health.Checker
}

func (r *delegatingReporter) add(err error) {
	r.Reporter.Add(&pb.Probe{
		Checker: r.checker.Name(),
		Error:   err.Error(),
		Status:  pb.Probe_Failed,
	})
}

func (r *delegatingReporter) addProbe(probe *pb.Probe) {
	probe.Checker = r.checker.Name()
	r.Reporter.Add(probe)
}
