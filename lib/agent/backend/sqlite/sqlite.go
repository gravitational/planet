package sqlite

import (
	"database/sql"
	"path/filepath"
	"time"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/jmoiron/sqlx"
	_ "github.com/gravitational/planet/Godeps/_workspace/src/github.com/mattn/go-sqlite3"
	"github.com/gravitational/planet/lib/agent/proto"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

const fileDb = "sqlite.db"

type backend struct {
	*sqlx.DB
}

/*
// Turn on foreign key enforcement (off by default since 3.6.19)
PRAGMA foreign_keys = ON
*/

const schema = `
CREATE TABLE IF NOT EXISTS node (
	id	INTEGER PRIMARY KEY NOT NULL,
	name	TEXT UNIQUE,
	-- active/left/unknown
	status	CHAR(1)	CHECK(status IN ('A', 'L', 'N')) NOT NULL DEFAULT 'A'
);

-- Keep the history of nodes becoming healthy.
CREATE TABLE IF NOT EXISTS node_health (
	node 	   PRIMARY KEY NOT NULL,
	healthy_at TIMESTAMP NOT NULL
);

-- composite ID: (node, checker, captured_at)
CREATE TABLE IF NOT EXISTS probe (
	node	    INTEGER NOT NULL,
	checker	    TEXT NOT NULL,
	extra 	    TEXT,
	-- running/failed
	status	    CHAR(1) CHECK(status IN ('H', 'F')) NOT NULL DEFAULT 'F',
	error	    TEXT NOT NULL,
	captured_at TIMESTAMP NOT NULL
);
`

func New(dataDir string) (*backend, error) {
	file := filepath.Join(dataDir, fileDb)
	db, err := sqlx.Open("sqlite3", file)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return newBackend(db)
}

func (r *backend) UpdateNode(status *pb.NodeStatus) (err error) {
	err = r.inTx(func(tx *sqlx.Tx) error {
		var id int64
		id, err = r.upsertNode(status.Name)
		if err != nil {
			return trace.Wrap(err)
		}
		err = r.addStatus(id, status)
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

func (r *backend) RecentStatus(node string) ([]*pb.Probe, error) {
	const selectStmt = `
	SELECT p.checker, p.extra, p.status, p.error, p.captured_at
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
		var timestamp time.Time
		var status string
		err = rows.Scan(&probe.Checker, &probe.Extra, &status, &probe.Error, &timestamp)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		probe.Timestamp = proto.TimeToProto(timestamp)
		probe.Status = statusFromString(status)
		probes = append(probes, &probe)
	}
	err = rows.Err()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return probes, nil
}

func (r *backend) Close() error {
	return r.DB.Close()
}

func (r *backend) upsertNode(node string) (id int64, err error) {
	const insertStmt = `INSERT OR IGNORE INTO node(name) VALUES(?)`
	var res sql.Result
	res, err = r.Exec(insertStmt, node)
	if err != nil {
		return 0, trace.Wrap(err)
	}
	id, _ = res.LastInsertId()
	if id == 0 {
		err = r.Get(&id, `SELECT id FROM node WHERE name=?`, node)
		if err != nil {
			return 0, trace.Wrap(err)
		}
	}
	return id, nil
}

func (r *backend) addStatus(node int64, status *pb.NodeStatus) (err error) {
	const insertStmt = `
		INSERT INTO probe(node, checker, extra, status, error, captured_at)
		VALUES(?, ?, ?, ?, ?, ?)
	`
	for _, probe := range status.Probes {
		_, err = r.Exec(insertStmt, node, probe.Checker, probe.Extra, serviceStatus(probe.Status).String(),
			probe.Error, proto.ProtoToTime(probe.Timestamp))
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

type serviceStatus pb.ServiceStatusType

func (r serviceStatus) String() string {
	switch pb.ServiceStatusType(r) {
	case pb.ServiceStatusType_ServiceRunning:
		return "H"
	case pb.ServiceStatusType_ServiceFailed:
		fallthrough
	default:
		return "F"
	}
}

func statusFromString(s string) pb.ServiceStatusType {
	switch s {
	case "H":
		return pb.ServiceStatusType_ServiceRunning
	case "F":
		fallthrough
	default:
		return pb.ServiceStatusType_ServiceFailed
	}
}

const scavengeTimeout = 24 * time.Hour

func (r *backend) scavengeLoop() {
	var timeout <-chan time.Time
	for {
		timeout = time.After(scavengeTimeout)
		select {
		case <-timeout:
			if err := r.deleteOlderThan(time.Now().Add(-scavengeTimeout)); err != nil {
				log.Errorf("failed to scavenge stats: %v", err)
			}
		}
	}
}

func (r *backend) deleteOlderThan(limit time.Time) error {
	const deleteStmt = `DELETE FROM probe WHERE captured_at < ?`

	err := r.inTx(func(tx *sqlx.Tx) error {
		_, err := r.Exec(deleteStmt, limit)
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

func (r *backend) inTx(f func(tx *sqlx.Tx) error) error {
	tx, err := r.Beginx()
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

func newInMemory() (*backend, error) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return newBackend(db)
}

func newBackend(db *sqlx.DB) (*backend, error) {
	_, err := db.Exec(schema)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create schema")
	}
	backend := &backend{DB: db}
	go backend.scavengeLoop()
	return backend, nil
}
