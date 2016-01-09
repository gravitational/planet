package sqlite

import (
	"database/sql"
	"path/filepath"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/jmoiron/sqlx"
	_ "github.com/gravitational/planet/Godeps/_workspace/src/github.com/mattn/go-sqlite3"
	"github.com/gravitational/planet/lib/agent/health"
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
	service     TEXT,
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
	_, err = db.Exec(schema)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create schema")
	}
	return &backend{DB: db}, nil
}

func (r *backend) AddStats(node string, stats *health.NodeStats) (err error) {
	var id int64
	var tx *sqlx.Tx
	tx, err = r.Beginx()
	if err != nil {
		return trace.Wrap(err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		err = tx.Commit()
	}()
	id, err = r.addNode(node)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("node: %d", id)
	err = r.addStats(id, stats)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (r *backend) Close() error {
	return r.DB.Close()
}

func (r *backend) addNode(node string) (id int64, err error) {
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

func (r *backend) addStats(node int64, stats *health.NodeStats) (err error) {
	const insertStmt = `
		INSERT INTO probe(node, checker, service, status, error, captured_at)
		VALUES(?, ?, ?, ?, ?, ?)
	`
	for _, probe := range stats.Probes {
		_, err = r.Exec(insertStmt, node, probe.Checker, probe.Service, statusType(probe.Status).String(),
			probe.Message, stats.Timestamp)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

type statusType health.StatusType

func (r statusType) String() string {
	switch health.StatusType(r) {
	case health.StatusRunning:
		return "H"
	case health.StatusFailed:
		fallthrough
	default:
		return "F"
	}
}
