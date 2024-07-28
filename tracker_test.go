package conntrack_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/peterbourgon/conntrack"
)

func TestDialer(t *testing.T) {
	t.Parallel()

	tracker := conntrack.NewTracker()
	events := make(chan string, 100)
	trackingDialer := tracker.NewDialer(&net.Dialer{}, conntrack.DialerConfig{
		OnDial: func(ctx context.Context, netw, addr string, c net.Conn, err error) {
			incrContextCounter(ctx)
			events <- fmt.Sprintf("OnDial %s %s (%s) -> %v", netw, addr, conntrack.SafeRemoteAddr(c), err)
		},
		OnClose: func(_ context.Context, c net.Conn, err error) {
			events <- fmt.Sprintf("OnClose (%s) -> %v", conntrack.SafeRemoteAddr(c), err)
		},
	})

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: trackingDialer.DialContext,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, time.Now().String())
	}))
	t.Cleanup(server.Close)

	serverAddr := server.Listener.Addr().String()

	ctx, c := withContextCounter(context.Background())

	req1, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	res1, err := client.Do(req1)
	if err != nil {
		t.Fatal(err)
	}

	if want, have := uint64(1), c.Load(); want != have {
		t.Fatalf("context counter: want %d, have %d", want, have)
	}

	req2, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	res2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}

	// req2 goes to the same server, so there isn't a new OnDial.
	if want, have := uint64(1), c.Load(); want != have {
		t.Fatalf("context counter: want %d, have %d", want, have)
	}

	if want, have := fmt.Sprintf("OnDial tcp %[1]s (%[1]s) -> <nil>", serverAddr), recvTimeout(t, events, time.Second); want != have {
		t.Errorf("event: want %q, have %q", want, have)
	}
	if want, have := fmt.Sprintf("OnDial tcp %[1]s (%[1]s) -> <nil>", serverAddr), recvTimeout(t, events, time.Second); want != have {
		t.Errorf("event: want %q, have %q", want, have)
	}
	if want, have := 2, len(tracker.Connections()); want != have {
		t.Errorf("tracker connections: want %d, have %d", want, have)
	}

	res2.Body.Close()

	if want, have := fmt.Sprintf("OnClose (%s) -> <nil>", serverAddr), recvTimeout(t, events, time.Second); want != have {
		t.Errorf("event: want %q, have %q", want, have)
	}

	if want, have := 1, len(tracker.Connections()); want != have {
		t.Errorf("tracker.Connections: want %d, have %d", want, have)
	}

	res1.Body.Close()

	if want, have := fmt.Sprintf("OnClose (%s) -> <nil>", serverAddr), recvTimeout(t, events, time.Second); want != have {
		t.Errorf("event: want %q, have %q", want, have)
	}

	if want, have := 0, len(tracker.Connections()); want != have {
		t.Errorf("tracker.Connections: want %d, have %d", want, have)
	}
}

func TestListener(t *testing.T) {
	t.Parallel()

	baseListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, c := withContextCounter(context.Background())

	tracker := conntrack.NewTracker()
	events := make(chan string, 100)
	trackingListener := tracker.NewListener(ctx, baseListener, conntrack.ListenerConfig{
		OnAccept: func(ctx context.Context, c net.Conn, err error) {
			incrContextCounter(ctx)
			events <- fmt.Sprintf("OnAccept %s %s -> %v", conntrack.SafeLocalAddr(c), conntrack.SafeRemoteAddr(c), err)
		},
		OnClose: func(_ context.Context, c net.Conn, err error) {
			events <- fmt.Sprintf("OnClose %s %s -> %v", conntrack.SafeLocalAddr(c), conntrack.SafeRemoteAddr(c), err)
		},
	})

	listenerAddr := trackingListener.Addr().String()

	errc := make(chan error, 1)
	go func() {
		for {
			c, err := trackingListener.Accept()
			if err != nil {
				errc <- err
				return
			}
			go func(c net.Conn) {
				io.Copy(io.Discard, c)
				c.Close()
			}(c)
		}
	}()
	defer func() {
		if err := trackingListener.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
		if err := <-errc; !(err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed)) {
			t.Errorf("accept loop: %v", err)
		}
	}()

	c1, err := net.Dial("tcp", trackingListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	c2, err := net.Dial("tcp", trackingListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	if want, have := fmt.Sprintf("OnAccept %s %s -> <nil>", listenerAddr, c1.LocalAddr()), recvTimeout(t, events, time.Second); want != have {
		t.Errorf("event: want %q, have %q", want, have)
	}

	if want, have := fmt.Sprintf("OnAccept %s %s -> <nil>", listenerAddr, c2.LocalAddr()), recvTimeout(t, events, time.Second); want != have {
		t.Errorf("event: want %q, have %q", want, have)
	}

	if want, have := 2, len(tracker.Connections()); want != have {
		t.Errorf("tracker.Connections: want %d, have %d", want, have)
	}

	if want, have := uint64(2), c.Load(); want != have {
		t.Fatalf("context counter: want %d, have %d", want, have)
	}

	if err := c2.Close(); err != nil {
		t.Fatalf("c2.Close: %v", err)
	}

	if want, have := fmt.Sprintf("OnClose %s %s -> <nil>", listenerAddr, c2.LocalAddr()), recvTimeout(t, events, time.Second); want != have {
		t.Errorf("event: want %q, have %q", want, have)
	}
	if want, have := 1, len(tracker.Connections()); want != have {
		t.Errorf("tracker.Connections: want %d, have %d", want, have)
	}

	if err := c1.Close(); err != nil {
		t.Fatalf("c1.Close: %v", err)
	}

	if want, have := fmt.Sprintf("OnClose %s %s -> <nil>", listenerAddr, c1.LocalAddr()), recvTimeout(t, events, time.Second); want != have {
		t.Errorf("event: want %q, have %q", want, have)
	}
	if want, have := 0, len(tracker.Connections()); want != have {
		t.Errorf("tracker.Connections: want %d, have %d", want, have)
	}
}

func BenchmarkTrackingOverhead(b *testing.B) {
	ctx := context.Background()

	drain := func(ln net.Listener) error {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		_, err = io.Copy(io.Discard, c)
		return err
	}

	for _, tc := range []struct {
		name string
		size int64
	}{
		{"1KB", 1 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1000 * 1024},
	} {
		b.Run(tc.name, func(b *testing.B) {
			packet := bytes.Repeat([]byte{'a'}, int(tc.size))

			b.Run("nothing", func(b *testing.B) {
				np := newNetpipe()
				ln := net.Listener(np)
				errc := make(chan error, 1)
				go func() { errc <- drain(ln) }()
				b.Cleanup(func() { np.Close(); <-errc })
				dl := np
				cc, _ := dl.DialContext(ctx, "", "")
				b.Cleanup(func() { cc.Close() })
				b.ResetTimer()
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					cc.Write(packet)
				}
			})

			b.Run("listener", func(b *testing.B) {
				np := newNetpipe()
				tr := conntrack.NewTracker()
				ln := tr.NewListener(ctx, net.Listener(np), conntrack.ListenerConfig{})
				errc := make(chan error, 1)
				go func() { errc <- drain(ln) }()
				b.Cleanup(func() { np.Close(); <-errc })
				dl := np
				cc, _ := dl.DialContext(ctx, "", "")
				b.Cleanup(func() { cc.Close() })
				b.ResetTimer()
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					cc.Write(packet)
				}
			})

			b.Run("dialer", func(b *testing.B) {
				np := newNetpipe()
				tr := conntrack.NewTracker()
				ln := net.Listener(np)
				errc := make(chan error, 1)
				go func() { errc <- drain(ln) }()
				b.Cleanup(func() { np.Close(); <-errc })
				dl := tr.NewDialContextFunc(np.DialContext, conntrack.DialerConfig{})
				cc, _ := dl.DialContext(ctx, "", "")
				b.Cleanup(func() { cc.Close() })
				b.ResetTimer()
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					cc.Write(packet)
				}
			})

			b.Run("both", func(b *testing.B) {
				np := newNetpipe()
				tr := conntrack.NewTracker()
				ln := tr.NewListener(ctx, net.Listener(np), conntrack.ListenerConfig{})
				errc := make(chan error, 1)
				go func() { errc <- drain(ln) }()
				b.Cleanup(func() { np.Close(); <-errc })
				dl := tr.NewDialContextFunc(np.DialContext, conntrack.DialerConfig{})
				cc, _ := dl.DialContext(ctx, "", "")
				b.Cleanup(func() { cc.Close() })
				b.ResetTimer()
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					cc.Write(packet)
				}
			})

		})
	}
}

//
//
//

func recvTimeout[T any](tb testing.TB, c <-chan T, timeout time.Duration) T {
	tb.Helper()
	select {
	case val := <-c:
		return val
	case <-time.After(timeout):
		tb.Errorf("timeout waiting for chan recv")
		var zero T
		return zero
	}
}

//
//
//

type netpipe struct {
	server net.Conn
	client net.Conn
}

func newNetpipe() *netpipe {
	server, client := net.Pipe()
	return &netpipe{server, client}
}

func (np *netpipe) Accept() (net.Conn, error) {
	return np.server, nil
}

func (np *netpipe) Close() error {
	return errors.Join(np.client.Close(), np.server.Close())
}

func (np *netpipe) Network() string {
	return "netpipe"
}

func (np *netpipe) String() string {
	return "netpipe"
}

func (np *netpipe) Addr() net.Addr {
	return np
}

func (np *netpipe) DialContext(context.Context, string, string) (net.Conn, error) {
	return np.client, nil
}

//
//
//

type contextCounterKey struct{}

func withContextCounter(ctx context.Context) (context.Context, *atomic.Uint64) {
	var c atomic.Uint64
	return context.WithValue(ctx, contextCounterKey{}, &c), &c
}

func incrContextCounter(ctx context.Context) {
	if c, ok := ctx.Value(contextCounterKey{}).(*atomic.Uint64); ok {
		c.Add(1)
	}
}
