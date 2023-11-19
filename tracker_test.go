package conntrack_test

import (
	"conntrack"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDialer(t *testing.T) {
	t.Parallel()

	tracker := conntrack.NewTracker()
	events := make(chan string, 100)
	trackingDialer := tracker.NewDialer(&net.Dialer{}, conntrack.DialerConfig{
		OnDial: func(netw, addr string, c net.Conn, err error) {
			events <- fmt.Sprintf("OnDial %s %s (%s) -> %v", netw, addr, c.RemoteAddr(), err)
		},
		OnClose: func(c net.Conn, err error) {
			events <- fmt.Sprintf("OnClose (%s) -> %v", c.RemoteAddr(), err)
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

	req1, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	res1, err := client.Do(req1)
	if err != nil {
		t.Fatal(err)
	}

	req2, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	res2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
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

	tracker := conntrack.NewTracker()
	events := make(chan string, 100)
	trackingListener := tracker.NewListener(baseListener, conntrack.ListenerConfig{
		OnAccept: func(c net.Conn, err error) {
			events <- fmt.Sprintf("OnAccept %s %s -> %v", conntrack.SafeLocalAddr(c), conntrack.SafeRemoteAddr(c), err)
		},
		OnClose: func(c net.Conn, err error) {
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
