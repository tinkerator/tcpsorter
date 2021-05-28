// Package tcpsorter provides a (net.Listener).Accept() abstraction
// that can Accept on a single socket multiple different protocols.
package tcpsorter

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Listener holds the tcpsorter's net.Listener interface
// implementation.
type Listener struct {
	prefixes [][]byte
	p        *Portal
	lis      chan connInfo
}

// Accept implements the (net.Listener).Accept() method.
func (lis *Listener) Accept() (net.Conn, error) {
	ci, ok := <-lis.lis
	if !ok {
		return nil, fmt.Errorf("%v closed", lis.p.Addr())
	}
	if ci.err == nil {
		var never time.Time
		ci.c.SetDeadline(never)
	}
	return ci.c, ci.err
}

// Close implements the (net.Listener).Close() method.
func (lis *Listener) Close() error {
	return lis.p.close(lis.prefixes...)
}

// Addr implements the (net.Listener).Addr() method.
func (lis *Listener) Addr() net.Addr {
	return lis.p.Addr()
}

// connInfo is used for passing the result of an Accept over a
// channel. We use it to redirect the immediately accepted connection
// to a specific Listener.
type connInfo struct {
	c   net.Conn
	err error
}

// TCPConn acts like a net.TCPConn, but it replays the prefix data
// before handing off to the raw net.TCPConn connection.
type TCPConn struct {
	tcp      *net.TCPConn
	partial  []byte
	consumed int
}

func (c *TCPConn) Close() error                             { return c.tcp.Close() }
func (c *TCPConn) CloseRead() error                         { c.consumed = len(c.partial); return c.tcp.CloseRead() }
func (c *TCPConn) CloseWrite() error                        { return c.tcp.CloseWrite() }
func (c *TCPConn) File() (f *os.File, err error)            { return c.tcp.File() }
func (c *TCPConn) LocalAddr() net.Addr                      { return c.tcp.LocalAddr() }
func (c *TCPConn) ReadFrom(r io.Reader) (int64, error)      { return c.tcp.ReadFrom(r) }
func (c *TCPConn) RemoteAddr() net.Addr                     { return c.tcp.RemoteAddr() }
func (c *TCPConn) SetDeadline(t time.Time) error            { return c.tcp.SetDeadline(t) }
func (c *TCPConn) SetKeepAlive(keepalive bool) error        { return c.tcp.SetKeepAlive(keepalive) }
func (c *TCPConn) SetKeepAlivePeriod(d time.Duration) error { return c.tcp.SetKeepAlivePeriod(d) }
func (c *TCPConn) SetLinger(sec int) error                  { return c.tcp.SetLinger(sec) }
func (c *TCPConn) SetNoDelay(noDelay bool) error            { return c.tcp.SetNoDelay(noDelay) }
func (c *TCPConn) SetReadBuffer(bytes int) error            { return c.tcp.SetReadBuffer(bytes) }
func (c *TCPConn) SetReadDeadline(t time.Time) error        { return c.tcp.SetReadDeadline(t) }
func (c *TCPConn) SetWriteBuffer(bytes int) error           { return c.tcp.SetWriteBuffer(bytes) }
func (c *TCPConn) SetWriteDeadline(t time.Time) error       { return c.tcp.SetWriteDeadline(t) }
func (c *TCPConn) SyscallConn() (syscall.RawConn, error)    { return c.tcp.SyscallConn() }
func (c *TCPConn) Write(b []byte) (int, error)              { return c.tcp.Write(b) }

func (c *TCPConn) Read(b []byte) (int, error) {
	n := len(c.partial) - c.consumed
	if n > 0 {
		if n >= len(b) {
			copy(b, c.partial[c.consumed:c.consumed+len(b)])
			c.consumed += len(b)
			return len(b), nil
		}
		copy(b, c.partial[c.consumed:])
		c.consumed = len(c.partial)
	}
	extras, err := c.tcp.Read(b[n:])
	if extras <= 0 && n != 0 {
		return n, nil
	}
	return extras + n, err
}

// Portal holds the port-listening TCP sorter.
type Portal struct {
	lis       net.Listener
	failover  chan connInfo
	mu        sync.Mutex
	listeners map[string]chan connInfo
}

// NewPortal creates a TCP port listener associated with a specific
// network port. Once initialized, add all sorted listeners via
// Listen() and then use Run() to actually start listening/sorting.
func NewPortal(network, port string) (*Portal, error) {
	lis, err := net.Listen(network, port)
	if err != nil {
		return nil, err
	}
	return &Portal{
		lis:       lis,
		failover:  make(chan connInfo),
		listeners: make(map[string]chan connInfo),
	}, nil
}

// startConn is launched via a goroutine to hand off a new connection
// to the configured Listener.
func (p *Portal) startConn(c net.Conn, partial []byte, ch chan connInfo) {
	if tcp, ok := c.(*net.TCPConn); ok {
		cc := &TCPConn{
			tcp:     tcp,
			partial: partial,
		}
		ch <- connInfo{c: cc}
		return
	}
	c.Close()
	ch <- connInfo{err: fmt.Errorf("%v is not a TCP connection", c.LocalAddr())}
}

// Addr redirects to the (*Portal).Addr().
func (p *Portal) Addr() net.Addr {
	return p.lis.Addr()
}

// close causes a Listener to be closed. This specifically closes all
// of the prefixes being watched for on this channel.
func (p *Portal) close(prefixes ...[]byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if prefixes == nil || prefixes[0] == nil {
		if p.failover != nil {
			close(p.failover)
			p.failover = nil
		}
		return nil
	}
	var lastCh chan connInfo
	for i := 0; i < len(prefixes); i++ {
		key := string(prefixes[i])
		if ch, ok := p.listeners[key]; ok {
			if lastCh != ch {
				close(ch)
				lastCh = ch
			}
			delete(p.listeners, key)
		}
	}
	return nil
}

// Listen returns a net.Listener for all new connections that have the
// first few bytes of incoming data equal to prefix. If the provided
// prefix is empty (or equivalently nil), this will return the default
// failover-listener.
func (p *Portal) Listen(prefixes ...[]byte) (net.Listener, error) {
	pl := &Listener{p: p, prefixes: prefixes}
	p.mu.Lock()
	defer p.mu.Unlock()
	if prefixes == nil || prefixes[0] == nil {
		pl.lis = p.failover
		if pl.lis == nil {
			return nil, net.ErrClosed
		}
	} else {
		keys := make(map[string]bool)
		for i := 0; i < len(prefixes); i++ {
			key := string(prefixes[i])
			for x := range p.listeners {
				if strings.HasPrefix(key, x) {
					return nil, fmt.Errorf("%q is a prefix for listener %q", x, key)
				}
				if strings.HasPrefix(x, key) {
					return nil, fmt.Errorf("%q is covered by listener %q", key, x)
				}
			}
			if _, reused := keys[key]; reused {
				return nil, fmt.Errorf("%q is a duplicate", key)
			}
			keys[key] = true
		}

		ch := make(chan connInfo)
		for key := range keys {
			p.listeners[key] = ch
		}
		pl.lis = ch
	}
	return pl, nil
}

// runner decodes enough of the incoming stream to sort it, or timeout trying.
func (p *Portal) runner(c net.Conn) {
	var partial []byte
	for hit := true; hit; {
		one := make([]byte, 1)
		if n, err := c.Read(one); n != 1 || err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				// Too late to service this stream.
				c.Close()
				return
			}
			p.startConn(c, partial, p.failover)
			return
		}
		partial = append(partial, one...)
		key := string(partial)
		p.mu.Lock()
		ci, ok := p.listeners[key]
		p.mu.Unlock()
		if ok {
			p.startConn(c, partial, ci)
			return
		}
		hit = false
		p.mu.Lock()
		for reserved := range p.listeners {
			if strings.HasPrefix(reserved, key) {
				hit = true
				break
			}
		}
		p.mu.Unlock()
		if !hit {
			p.startConn(c, partial, p.failover)
			return
		}
	}
}

// Run causes the Portal to start accepting and sorting connections
// until the Portal is Close()d.
func (p *Portal) Run(timeout time.Duration) error {
	for {
		c, err := p.lis.Accept()
		if err != nil {
			done := make(map[chan connInfo]bool)
			p.mu.Lock()
			for key, ch := range p.listeners {
				if !done[ch] {
					close(ch)
					done[ch] = true
				}
				delete(p.listeners, key)
			}
			if p.failover != nil {
				close(p.failover)
			}
			p.mu.Unlock()
			return err
		}
		// Give client only so long to be sorted.
		c.SetDeadline(time.Now().Add(timeout))
		go p.runner(c)
	}
}

// Close closes the Portal and ceases all listening. Note, all open
// connections need to be closed independently.
func (p *Portal) Close() error {
	return p.lis.Close()
}
