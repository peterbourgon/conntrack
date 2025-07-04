package conntrack

import (
	"context"
	"net"
)

// Listener is a [net.Listener] that tracks accepted connections.
type Listener struct {
	net.Listener

	ctx      context.Context
	tracker  *Tracker
	onAccept func(context.Context, net.Conn, error)
	config   connConfig
}

func newListener(ctx context.Context, ln net.Listener, t *Tracker, c ListenerConfig) *Listener {
	c.validate()
	return &Listener{
		Listener: ln,
		tracker:  t,
		ctx:      ctx,
		onAccept: c.OnAccept,
		config:   c.connConfig(),
	}
}

// Accept decorates the net.Listener method for tracking purposes.
func (ln *Listener) Accept() (conn net.Conn, err error) {
	defer func() {
		ln.onAccept(ln.ctx, conn, err)
	}()

	conn, err = ln.Listener.Accept()
	if err == nil && conn != nil {
		conn = ln.tracker.newConn(ln.ctx, conn, ln.config, "server")
	}

	return conn, err
}

// ListenerConfig captures the config parameters for a tracking Listener.
type ListenerConfig struct {
	// Category is included in the connection info for every connection created
	// from this listener.
	Category string

	// OnAccept is an optional callback that, if non-nil, will be called at the
	// end of every accept operation made by the listener.
	OnAccept func(ctx context.Context, c net.Conn, err error)

	// OnRead is an optional callback that, if non-nil, will be called at the
	// end of every read operation made on any connection created from the
	// listener.
	OnRead func(ctx context.Context, n int, err error)

	// OnWrite is an optional callback that, if non-nil, will be called at the
	// end of every write operation made on any connection created from the
	// listener.
	OnWrite func(ctx context.Context, n int, err error)

	// OnClose is an optional callback that, if non-nil, will be called whenever
	// a connection created from the listener is closed.
	OnClose func(ctx context.Context, c net.Conn, err error)
}

func (c *ListenerConfig) validate() {
	if c.OnAccept == nil {
		c.OnAccept = func(context.Context, net.Conn, error) {}
	}

	if c.OnRead == nil {
		c.OnRead = func(context.Context, int, error) {}
	}

	if c.OnWrite == nil {
		c.OnWrite = func(context.Context, int, error) {}
	}

	if c.OnClose == nil {
		c.OnClose = func(context.Context, net.Conn, error) {}
	}
}

func (c *ListenerConfig) connConfig() connConfig {
	return connConfig{
		Category: c.Category,
		OnRead:   c.OnRead,
		OnWrite:  c.OnWrite,
		OnClose:  c.OnClose,
	}
}
