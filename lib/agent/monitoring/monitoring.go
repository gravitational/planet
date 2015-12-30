// package check abstracts a process of running health checks.
package monitoring

// Checker defines an obligation to run a health check.
type Checker interface {
	// Check runs a health check and records any errors into the specified reporter.
	Check(Reporter)
}

// Reporter defines an obligation to report errors with a specified name.
type Reporter interface {
	Add(name string, err error)
	AddEvent(event Event)
	Status() NodeStatus
}

type checker interface {
	// Check runs a health check and records any errors into the specified reporter.
	check(reporter)
}

// reporter defines an obligation to report errors.
type reporter interface {
	add(error)
	addEvent(Event)
}

type Tags map[string][]string

// Tester describes an instance of a health checker.
type Tester struct {
	checker
	Tags Tags
	Name string
}

// List of registered checkers.
var Testers []Tester

// AddChecker registers a new checker specified by name and a set of tags.
//
// Tags can be used to annotate a checker with a set of labels.
// For instance, checkers can easily be bound to a certain agent (and thus,
// a certain node) by starting an agent with the same set of tags as those
// specified by the checker and the checker will only run on that agent.
func AddChecker(checker checker, name string, tags Tags) {
	Testers = append(Testers, Tester{checker: checker, Name: name, Tags: tags})
}

func (r *Tester) Check(reporter Reporter) {
	rep := &delegatingReporter{Reporter: reporter, tester: r}
	r.check(rep)
}

type defaultReporter struct {
	status NodeStatus
}

func NewDefaultReporter(name string) Reporter {
	return &defaultReporter{status: NodeStatus{Name: name}}
}

// delegatingReporter binds a tester to an external reporter.
type delegatingReporter struct {
	Reporter
	tester *Tester
}

func (r *delegatingReporter) add(err error) {
	r.Reporter.Add(r.tester.Name, err)
}

func (r *delegatingReporter) addEvent(event Event) {
	event.Name = r.tester.Name
	r.Reporter.AddEvent(event)
}

func (r *defaultReporter) Add(name string, err error) {
	r.status.Events = append(r.status.Events, Event{
		Name:    name,
		Message: err.Error(),
		Status:  StatusFailed,
	})
}

func (r *defaultReporter) AddEvent(event Event) {
	r.status.Events = append(r.status.Events, event)
}

func (r *defaultReporter) Status() NodeStatus {
	return r.status
}
