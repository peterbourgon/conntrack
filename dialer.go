package conntrack

import (
	"context"
	"net"
	"time"
)

// Dialer is (essentially) a [net.Dialer] that tracks dialed connections.
type Dialer struct {
	dialFunc DialContextFunc
	tracker  *Tracker
	onDial   func(string, string, net.Conn, error)
	config   connConfig
}

// DialContextFunc models the DialContext method of a net.Dialer.
type DialContextFunc func(context.Context, string, string) (net.Conn, error)

func newDialer(f DialContextFunc, t *Tracker, c DialerConfig) *Dialer {
	c.validate()
	return &Dialer{
		dialFunc: f,
		tracker:  t,
		onDial:   c.OnDial,
		config:   c.connConfig(),
	}
}

// DialContext decorates the net.Dialer method for tracking purposes.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (conn net.Conn, err error) {
	defer func(begin time.Time) {
		d.onDial(network, address, conn, err)
	}(time.Now())

	conn, err = d.dialFunc(ctx, network, address)
	if err == nil && conn != nil {
		conn = d.tracker.newConn(conn, d.config, "client")
	}

	return conn, err
}

// DialerConfig captures the config parameters for a tracking Dialer.
type DialerConfig struct {
	// Category is included in the connection info for every connection created
	// from this dialer.
	Category string

	// OnDial is an optional callback that, if non-nil, will be called at the
	// end of every dial operation made by the dialer.
	OnDial func(netw string, addr string, c net.Conn, err error)

	// OnRead is an optional callback that, if non-nil, will be called at the
	// end of every read operation made on any connection created from the
	// dialer.
	OnRead func(n int, err error)

	// OnWrite is an optional callback that, if non-nil, will be called at the
	// end of every write operation made on any connection created from the
	// dialer.
	OnWrite func(n int, err error)

	// OnClose is an optional callback that, if non-nil, will be called whenever
	// a connection created from the dialer is closed.
	OnClose func(c net.Conn, err error)
}

func (c *DialerConfig) validate() {
	if c.OnDial == nil {
		c.OnDial = func(string, string, net.Conn, error) {}
	}

	if c.OnRead == nil {
		c.OnRead = func(int, error) {}
	}

	if c.OnWrite == nil {
		c.OnWrite = func(int, error) {}
	}

	if c.OnClose == nil {
		c.OnClose = func(net.Conn, error) {}
	}
}

func (c *DialerConfig) connConfig() connConfig {
	return connConfig{
		Category: c.Category,
		OnRead:   c.OnRead,
		OnWrite:  c.OnWrite,
		OnClose:  c.OnClose,
	}
}
