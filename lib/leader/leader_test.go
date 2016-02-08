package leader

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/client"
	"github.com/pborman/uuid"
	"golang.org/x/net/context"
	. "gopkg.in/check.v1"
)

func TestLeader(t *testing.T) { TestingT(t) }

type LeaderSuite struct {
	nodes []string
}

var _ = Suite(&LeaderSuite{})

func (s *LeaderSuite) SetUpSuite(c *C) {
	nodesString := os.Getenv("PLANET_TEST_ETCD_NODES")
	if nodesString == "" {
		// Skips the entire suite
		c.Skip("This test requires etcd, provide comma separated nodes in PLANET_TEST_ETCD_NODES environment variable")
		return
	}
	s.nodes = strings.Split(nodesString, ",")
	log.Initialize("console", "INFO")
}

func (s *LeaderSuite) newClient(c *C) *Client {
	clt, err := NewClient(Config{EtcdEndpoints: s.nodes, Timeout: 100 * time.Millisecond})
	c.Assert(err, IsNil)
	return clt
}

func (s *LeaderSuite) closeClient(c *C, clt *Client) {
	err := clt.Close()
	c.Assert(err, IsNil)
}

func (s *LeaderSuite) TestLeaderElectSingle(c *C) {
	clt := s.newClient(c)
	defer s.closeClient(c, clt)

	key := fmt.Sprintf("/planet/tests/elect/%v", uuid.New())

	changeC := make(chan string)
	clt.AddWatchCallback(key, 50*time.Millisecond, func(key, prevVal, newVal string) {
		changeC <- newVal
	})
	clt.AddVoter(key, "node1", time.Second)

	select {
	case val := <-changeC:
		c.Assert(val, Equals, "node1")
	case <-time.After(time.Second):
		c.Fatalf("timeout waiting for event")
	}
}

func (s *LeaderSuite) TestLeaderTakeover(c *C) {
	clta := s.newClient(c)

	cltb := s.newClient(c)
	defer s.closeClient(c, cltb)

	key := fmt.Sprintf("/planet/tests/elect/%v", uuid.New())

	changeC := make(chan string)
	cltb.AddWatchCallback(key, 50*time.Millisecond, func(key, prevVal, newVal string) {
		changeC <- newVal
	})
	clta.AddVoter(key, "voter a", time.Second)

	// make sure we've elected voter a
	select {
	case val := <-changeC:
		c.Assert(val, Equals, "voter a")
	case <-time.After(time.Second):
		c.Fatalf("timeout waiting for event")
	}

	// add voter b to the equation
	cltb.AddVoter(key, "voter b", time.Second)

	// now, shut down voter a
	c.Assert(clta.Close(), IsNil)
	// in a second, we should see the leader has changed
	time.Sleep(time.Second)

	// make sure we've elected voter b
	select {
	case val := <-changeC:
		c.Assert(val, Equals, "voter b")
	case <-time.After(time.Second):
		c.Fatalf("timeout waiting for event")
	}
}

// Make sure leader extends lease in time
func (s *LeaderSuite) TestLeaderExtendLease(c *C) {
	clt := s.newClient(c)
	defer s.closeClient(c, clt)

	key := fmt.Sprintf("/planet/tests/elect/%v", uuid.New())
	clt.AddVoter(key, "voter a", time.Second)
	time.Sleep(900 * time.Millisecond)

	api := client.NewKeysAPI(clt.client)
	re, err := api.Get(context.TODO(), key, nil)
	c.Assert(err, IsNil)
	expiresIn := re.Node.Expiration.Sub(time.Now())
	maxTTL := 500 * time.Millisecond
	c.Assert(expiresIn > maxTTL, Equals, true, Commentf("%v > %v", expiresIn, maxTTL))
}
