package ql

import (
	"path/filepath"
	"time"

	_ "github.com/gravitational/planet/Godeps/_workspace/src/github.com/cznic/ql"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/lib/agent/health"
)

type Node struct {
	ID     int    `ql:"index xID"`
	Name   string `ql:"name"`
	Status string `ql:"status"`
}

type NodeHealth struct {
	Node   int    `ql:"node"`
	Status string `ql:"status"`
}

type Probe struct {
	NodeID    int       `ql:"node"`
	Checker   string    `ql:"checker,index xChecker"`
	Service   string    `ql:"service"`
	Status    string    `ql:"status"`
	Error     string    `ql:"error"`
	Timestamp time.Time `ql:"captured_at"`
}

const fileDb = "ql.db"

// FIXME: use ql.Schema for schema definition
const schema = `
BEGIN TRANSACTION;

CREATE TABLE IF NOT EXISTS node (
	id 	int,
	name	string,
	// active/left/inaccessible
	status	string
);

// Keeps the history of nodes becoming healthy.
CREATE TABLE IF NOT EXISTS node_health (
	node 	   int not null,
	healthy_at time not null
);

// composite ID: (node, checker, captured_at)
CREATE TABLE IF NOT EXISTS probe (
	node	    int not null,
	checker	    string not null,
	service     string,
	status	    string not null,
	error	    string not null,
	captured_at time not null
);

// TODO: create indices if required

COMMIT;
`

// backend.Backend
type driver struct {
	*ql.DB
}

func (r *driver) AddStats(stats *health.NodeStats) error {
	// TODO
	return nil
}

func (r *driver) Close() error {
	return r.DB.Close()
}

func New(dataDir string) (*driver, error) {
	db, err := ql.OpenFile(filepath.Join(dataDir, fileDb), ql.Options{CanCreate: true})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	ctx := ql.NewRWCtx()
	_, _, err = db.Run(ctx, schema)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create schema")
	}
	return &driver{DB: db}, nil
}
