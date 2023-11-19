package conntrack

import (
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Tracker keeps track of connections.
//
// To track incoming connections, whenever you create a [net.Listener], pass it
// to [Tracker.NewListener], and use the returned Listener instead.
//
// To track outgoing connections, whenever you create a [net.Dialer], pass it to
// [Tracker.NewDialer], and use the returned Dialer instead.
type Tracker struct {
	mtx   sync.Mutex
	conns map[*Conn]struct{}
}

// NewTracker returns a fresh connection tracker.
func NewTracker() *Tracker {
	return &Tracker{
		conns: map[*Conn]struct{}{},
	}
}

// NewListener decorates the given net.Listener so that the connections it
// accepts are tracked by the tracker.
func (t *Tracker) NewListener(ln net.Listener, c ListenerConfig) *Listener {
	return newListener(ln, t, c)
}

// NewDialer decorates the net.Dialer so that the connections it creates are
// tracked by the tracker. It's equivalent to calling NewDialContextFunc with
// d.DialContext.
func (t *Tracker) NewDialer(d *net.Dialer, c DialerConfig) *Dialer {
	return t.NewDialContextFunc(d.DialContext, c)
}

// NewDialContextFunc decorates the DialContextFunc so that the connections it
// creates are tracked by the tracker.
func (t *Tracker) NewDialContextFunc(f DialContextFunc, c DialerConfig) *Dialer {
	return newDialer(f, t, c)
}

// Connections returns metadata about all currently active connections.
func (t *Tracker) Connections() []ConnInfo {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	var (
		now   = time.Now()
		infos = make([]ConnInfo, 0, len(t.conns))
	)
	for c := range t.conns {
		infos = append(infos, ConnInfo{
			Category:       c.config.Category,
			ClientServer:   c.clientServer,
			LocalAddr:      c.Conn.LocalAddr().String(),
			RemoteAddr:     c.Conn.RemoteAddr().String(),
			EstablishedFor: now.Sub(c.createdAt),
			ReadBytes:      atomic.LoadUint64(&c.rd),
			WriteBytes:     atomic.LoadUint64(&c.wr),
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		a := infos[i].Category + infos[i].ClientServer + infos[i].RemoteAddr + infos[i].LocalAddr
		b := infos[j].Category + infos[j].ClientServer + infos[j].RemoteAddr + infos[j].LocalAddr
		return a < b
	})

	return infos
}

func (t *Tracker) newConn(conn net.Conn, config connConfig, clientServer string) *Conn {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	c := newConn(conn, t, config, clientServer)
	t.conns[c] = struct{}{}
	return c
}

func (t *Tracker) closeConn(c *Conn, err error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	delete(t.conns, c)
}
