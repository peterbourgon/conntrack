package conntrack

import (
	"net"
	"sync/atomic"
	"time"
)

// Conn is a tracked [net.Conn], created by a [Listener] or [Dialer].
type Conn struct {
	net.Conn

	tracker      *Tracker
	config       connConfig
	clientServer string
	createdAt    time.Time
	rd, wr       uint64
}

func newConn(conn net.Conn, t *Tracker, config connConfig, clientServer string) *Conn {
	return &Conn{
		Conn:         conn,
		tracker:      t,
		config:       config,
		clientServer: clientServer,
		createdAt:    time.Now().UTC(),
	}
}

// Read decorates the net.Conn method for tracking purposes.
func (c *Conn) Read(b []byte) (n int, err error) {
	defer func() {
		atomic.AddUint64(&c.rd, uint64(n))
		c.config.OnRead(n, err)
	}()

	return c.Conn.Read(b)
}

// Write decorates the net.Conn method for tracking purposes.
func (c *Conn) Write(b []byte) (n int, err error) {
	defer func() {
		atomic.AddUint64(&c.wr, uint64(n))
		c.config.OnWrite(n, err)
	}()

	return c.Conn.Write(b)
}

// Close decorates the net.Conn method for tracking purposes.
func (c *Conn) Close() (err error) {
	defer func() {
		c.tracker.closeConn(c, err)
		c.config.OnClose(c, err)
	}()

	return c.Conn.Close()
}

type connConfig struct {
	Category string
	OnRead   func(int, error)
	OnWrite  func(int, error)
	OnClose  func(net.Conn, error)
}

//
//
//

// ConnInfo is point-in-time metadata about a tracked connection.
type ConnInfo struct {
	// Category of the Dialer or Listener which created this connection.
	Category string

	// ClientServer is either "client" (when the connection is from a dialer) or
	// "server" (when the connection is from a listener).
	ClientServer string

	// LocalAddr is the local address of the connection.
	LocalAddr string

	// RemoteAddr is the remote address of the connection.
	RemoteAddr string

	// EstablishedFor is how long the connection has existed.
	EstablishedFor time.Duration

	// ReadBytes is how many bytes have been read from the connection.
	ReadBytes uint64

	// WriteBytes is how many bytes have been written to the connection.
	WriteBytes uint64
}

// Summarize returns a ConnSummary representing the provided ConnInfos.
func Summarize(cis []ConnInfo) ConnSummary {
	var cs ConnSummary
	for _, ci := range cis {
		cs.Add(ci)
	}
	return cs
}

// ConnSummary represents summary metadata for multiple ConnInfos.
type ConnSummary struct {
	// Count is how many connections are represented by the summary.
	Count int

	// Shortest connection established time.
	Shortest time.Duration

	// Longest connection established time.
	Longest time.Duration

	// ReadBytes is the total bytes read from all connections.
	ReadBytes uint64

	// WriteBytes is the total bytes written to all connections.
	WriteBytes uint64
}

// Add the connection info to the summary.
func (cs *ConnSummary) Add(ci ConnInfo) {
	cs.Count++
	if cs.Shortest == 0 || ci.EstablishedFor < cs.Shortest {
		cs.Shortest = ci.EstablishedFor
	}
	if cs.Longest == 0 || ci.EstablishedFor > cs.Longest {
		cs.Longest = ci.EstablishedFor
	}
	cs.ReadBytes += ci.ReadBytes
	cs.WriteBytes += ci.WriteBytes
}

//
//
//

// SafeLocalAddr returns c.LocalAddr().String(), or "<nil>" if c is nil.
func SafeLocalAddr(c net.Conn) string {
	if c == nil {
		return "<nil>"
	}
	return c.LocalAddr().String()
}

// SafeRemoteAddr returns c.RemoteAddr().String(), or "<nil>" if c is nil.
func SafeRemoteAddr(c net.Conn) string {
	if c == nil {
		return "<nil>"
	}
	return c.RemoteAddr().String()
}
