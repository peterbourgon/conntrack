# conntrack [![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/peterbourgon/conntrack) [![Latest Release](https://img.shields.io/github/v/release/peterbourgon/conntrack?style=flat-square)](https://github.com/peterbourgon/conntrack/releases/latest) ![Build Status](https://github.com/peterbourgon/conntrack/actions/workflows/test.yaml/badge.svg?branch=main)

Package conntrack allows Go programs to keep track of active connections.

First, create a conntrack.Tracker.

```go
tracker := conntrack.NewTracker()
```

To track incoming connections to a server, create and wrap a net.Listener.

```go
baseListener, err := net.Listen("tcp", "127.0.0.1:8080")
// handle err
trackingListener := tracker.NewListener(baseListener, conntrack.ListenerConfig{
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
baseDialer := &net.Dialer{...}
trackingDialer := tracker.NewDialer(baseDialer, conntrack.DialerConfig{
	Category: "optional-category",
	OnDial:   func(netw, addr string, c net.Conn, err error) { /* optional callback */ },
	OnRead:   func(n int, err error)                         { /* optional callback */ },
	OnWrite:  func(n int, err error)                         { /* optional callback */ },
	OnClose:  func(c net.Conn, err error)                    { /* optional callback */ },
})
client := &http.Client{
	Transport: &http.Transport{
		DialContext: trackingDialer.DialContext,
	},
}
res, err := client.Get("http://zombo.com")
// ...
```

The conntrack.Tracker maintains metadata for each active connection.

```go
infos := tracker.Connections()
for i, info := range infos {
	fmt.Printf("%d/%d\n", i+1, len(infos))
	fmt.Printf(" Category:       %s\n", info.Category)
	fmt.Printf(" ClientServer:   %s\n", info.ClientServer)
	fmt.Printf(" LocalAddr:      %s\n", info.LocalAddr)
	fmt.Printf(" RemoteAddr:     %s\n", info.RemoteAddr)
	fmt.Printf(" EstablishedFor: %s\n", info.EstablishedFor)
	fmt.Printf(" ReadBytes:      %d\n", info.ReadBytes)
	fmt.Printf(" WriteBytes:     %d\n", info.WriteBytes)
}
```
