package sqlite

import (
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/gravitational/log"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	_ "github.com/mattn/go-sqlite3"
)

type backend struct {
	*sql.DB
	clock clockwork.Clock
	done  chan struct{}
}

// TODO: store checkers in a separate table

const schema = `
PRAGMA foreign_keys = TRUE;

CREATE TABLE IF NOT EXISTS node (
	id INTEGER PRIMARY KEY NOT NULL,
	name TEXT UNIQUE,
	-- active/left/failed
	status CHAR(1)	CHECK(status IN ('A', 'L', 'F')) NOT NULL DEFAULT 'A'
);

-- system status snapshot
CREATE TABLE IF NOT EXISTS system_status (
)

CREATE TABLE IF NOT EXISTS checker (
	id   INTEGER PRIMARY KEY NOT NULL,
	name TEXT UNIQUE,
	desc TEXT
);

-- history of monitoring test results for a node
CREATE TABLE IF NOT EXISTS probe (
	node	    INTEGER NOT NULL,
	checker	    TEXT NOT NULL,
	detail	    TEXT,
	-- healthy/failed/terminated
	status	    CHAR(1) CHECK(status IN ('H', 'F', 'T')) NOT NULL DEFAULT 'F',
	error	    TEXT NOT NULL,
	captured_at TIMESTAMP NOT NULL,
	FOREIGN KEY(node) REFERENCES node(id)
);
`

// New creates a new sqlite backend using the specified file.
func New(path string) (*backend, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	clock := clockwork.NewRealClock()
	return newBackend(db, clock)
}

// UpdateNode will update the status of the node specified by status.
func (r *backend) UpdateNode(status *pb.NodeStatus) (err error) {
	if err = inTx(r.DB, func(tx *sql.Tx) error {
		err = addStatus(tx, status)
		if err != nil {
			return trace.Wrap(err)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// Update will update the status of all nodes part of the specified system status.
func (r *backend) Update(status *pb.SystemStatus) (err error) {
	if err = inTx(r.DB, func(tx *sql.Tx) error {
		for _, node := range status.Nodes {
			err = addStatus(tx, node)
			if err != nil {
				return trace.Wrap(err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// RecentStatus obtains the last known status for the specified node.
func (r *backend) RecentStatus(node string) (*pb.NodeStatus, error) {
	const selectStmt = `
	SELECT p.checker, p.detail, p.status, p.error, p.captured_at
	FROM probe p JOIN node n WHERE p.node = n.id AND n.name = ?
	ORDER BY p.captured_at DESC
	LIMIT 5
	`
	rows, err := r.Query(selectStmt, node)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer rows.Close()

	var probes []*pb.Probe
	for rows.Next() {
		probe := pb.Probe{}
		var when timestamp
		var status string
		err = rows.Scan(&probe.Checker, &probe.Detail, &status, &probe.Error, &when)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		probe.Timestamp = (*pb.Timestamp)(&when)
		probe.Status = probeType(status).toProto()
		probes = append(probes, &probe)
	}
	err = rows.Err()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	status := &pb.NodeStatus{Name: node, Probes: probes}
	return status, nil
}

// Close closes the database.
func (r *backend) Close() error {
	close(r.done)
	return r.DB.Close()
}

func addStatus(tx *sql.Tx, status *pb.NodeStatus) (err error) {
	const insertStmt = `
		INSERT OR IGNORE INTO node(name) VALUES(?);
		INSERT INTO probe(node, checker, detail, status, error, captured_at)
		SELECT n."rowid", ?, ?, ?, ?, ? FROM node n WHERE n.name=?
	`
	for _, probe := range status.Probes {
		_, err = tx.Exec(insertStmt, status.Name, probe.Checker, probe.Detail, protoToStatus(probe.Status),
			probe.Error, timestamp(*probe.Timestamp), status.Name)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

type probeType string

func protoToStatus(status pb.Probe_Type) probeType {
	switch status {
	case pb.Probe_Running:
		return probeType("H")
	case pb.Probe_Terminated:
		return probeType("T")
	default:
		return probeType("F")
	}
}

func (r probeType) toProto() pb.Probe_Type {
	switch r {
	case "H":
		return pb.Probe_Running
	case "T":
		return pb.Probe_Terminated
	default:
		return pb.Probe_Failed
	}
}

// driver.Valuer
func (r probeType) Value() (value driver.Value, err error) {
	return string(r), nil
}

type timestamp pb.Timestamp

// sql.Scanner
func (ts *timestamp) Scan(src interface{}) error {
	return (*pb.Timestamp)(ts).UnmarshalText(src.([]byte))
}

// driver.Valuer
func (ts timestamp) Value() (value driver.Value, err error) {
	return pb.Timestamp(ts).MarshalText()
}

const scavengeTimeout = 24 * time.Hour

func (r *backend) scavengeLoop() {
	for {
		select {
		case <-r.clock.After(scavengeTimeout):
			if err := r.deleteOlderThan(r.clock.Now().Add(-scavengeTimeout)); err != nil {
				log.Errorf("failed to scavenge stats: %v", err)
			}
		case <-r.done:
			return
		}
	}
}

func (r *backend) deleteOlderThan(limit time.Time) error {
	const deleteStmt = `DELETE FROM probe WHERE captured_at < ?`

	err := inTx(r.DB, func(tx *sql.Tx) error {
		_, err := tx.Exec(deleteStmt, timestamp(pb.TimeToProto(limit)))
		if err != nil {
			return trace.Wrap(err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

type cleanup func()

func inTx(db *sql.DB, f func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return trace.Wrap(err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		tx.Commit()
	}()
	err = f(tx)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func newInMemory(clock clockwork.Clock) (*backend, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return newBackend(db, clock)
}

func newBackend(db *sql.DB, clock clockwork.Clock) (*backend, error) {
	_, err := db.Exec(schema)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create schema")
	}

	backend := &backend{
		DB:    db,
		clock: clock,
		done:  make(chan struct{}),
	}
	go backend.scavengeLoop()
	return backend, nil
}
