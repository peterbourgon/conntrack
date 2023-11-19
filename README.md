# conntrack [![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/peterbourgon/conntrack) [![Latest Release](https://img.shields.io/github/v/release/peterbourgon/conntrack?style=flat-square)](https://github.com/peterbourgon/conntrack/releases/latest) ![Build Status](https://github.com/peterbourgon/conntrack/actions/workflows/test.yaml/badge.svg?branch=main)

Package conntrack allows Go programs to track incoming and outgoing connections.

To track incoming connections to a server, create and wrap a net.Listener.

```go
tracker := conntrack.NewTracker()
baseListener, err := net.Listen("tcp", "127.0.0.1:8080")
// handle err
trackingListener := tracker.Listener(baseListener, conntrack.ListenerConfig{
	Category: "optional-category",
	OnAccept: func(c net.Conn, err error) { /* optional callback */ },
	OnRead:   func(n int, err error)      { /* optional callback */ },
	OnWrite:  func(n int, err error)      { /* optional callback */ },
	OnClose:  func(c net.Conn, err error) { /* optional callback */ },
})
server := &http.Server{...}
server.Serve(trackingListener)
```

To track outgoing connections from a client, create and wrap a net.Dialer.

```go
tracker := conntrack.NewTracker()
baseDialer := &net.Dialer{...}
trackingDialer := tracker.Dialer(baseDialer, conntrack.DialerConfig{
	Category: "optional-category",
	OnDial:   func(netw, addr string, c net.Conn, err error) { /* optional callback */ },
	OnRead:   func(n int, err error)                         { /* optional callback */ },
	OnWrite:  func(n int, err error)                         { /* optional callback */ },
	OnClose:  func(c net.Conn, err error)                    { /* optional callback */ },
})
client := &http.Client{
	Transport: &http.Transport{
		DialContext: trackingDialer.DialContext,
	}
}
res, err := client.Get("http://zombo.com")
// ...
```

The tracker.Connections method returns metadata for all active connections.

```go
infos := tracker.Connections()
for i, info := range infos {
	fmt.Printf("%d/%d: %+v\n", i+1, len(infos), info)
}

// Output:
// 1/2: {Category:cat1 ClientServer:client LocalAddr:... RemoteAddr:... EstablishedFor:149ms ReadBytes:129 WriteBytes:96}
// 2/2: {Category:cat2 ClientServer:server LocalAddr:... RemoteAddr:... EstablishedFor:32.1s ReadBytes:125 WriteBytes:32}
```
