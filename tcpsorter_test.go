package tcpsorter

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestPortal(t *testing.T) {
	addr := "localhost"
	p, err := NewPortal("tcp", addr+":0")
	if err != nil {
		t.Fatalf("failed to listen to %q: %v", addr, err)
	}
	actual := p.Addr().String()

	t.Logf("opened listener to %q", actual)

	p0, err := p.Listen(nil)
	if err != nil {
		t.Fatalf("failed to listen for defaults: %v", err)
	}
	p1, err := p.Listen([]byte("abc"))
	if err != nil {
		t.Fatalf("failed to listen for \"abc\": %v", err)
	}
	if _, err := p.Listen([]byte("abc")); err == nil {
		t.Fatal("listening twice to \"abc\"")
	}
	if err := p1.Close(); err != nil {
		t.Fatalf("unable to close \"abc\" Listener: %v", err)
	}

	p1, err = p.Listen([]byte("abc"))
	if err != nil {
		t.Fatalf("failed to listen again for \"abc\": %v", err)
	}
	if _, err = p.Listen([]byte("abcd")); err == nil {
		t.Fatalf("didn't notice \"abc\" is ambiguous")
	}

	p2, err := p.Listen([]byte("abd"))
	if err != nil {
		t.Fatalf("failed to listen for \"abd\": %v", err)
	}

	var wg sync.WaitGroup

	listener := func(lis net.Listener, result string) {
		defer wg.Done()
		defer lis.Close()
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			io.ReadAll(c)
			fmt.Fprint(c, result)
			c.Close()
		}
	}

	wg.Add(3)
	go listener(p0, "zero")
	go listener(p1, "one")
	go listener(p2, "two")

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := p.Run(1 * time.Minute)
		if err == nil {
			t.Error("Run ended without error")
		}
	}()

	var streams = []struct {
		send string
		recv string
	}{
		{"abcf", "one"},
		{"abde", "two"},
		{"a", "zero"},
		{"abcd", "one"},
		{"dcba", "zero"},
	}
	var ds []net.Conn
	for i := range streams {
		s := streams[i]
		d, err := net.Dial("tcp", actual)
		if err != nil {
			t.Fatalf("stream %d %q failed dial: %v", i, s.send, err)
		}
		fmt.Fprint(d, s.send)
		d.(*net.TCPConn).CloseWrite()
		ds = append(ds, d)
	}

	for i := range ds {
		d := ds[i]
		b, err := io.ReadAll(d)
		if err != nil {
			t.Errorf("failed to read stream %d: %v", i, err)
		}
		if got, want := string(b), streams[i].recv; got != want {
			t.Errorf("stream %d returned %q, want %q", i, got, want)
		}
		if err := d.Close(); err != nil {
			t.Errorf("failed to close stream %d: %v", i, err)
		}
		t.Logf("test %d", i)
	}

	if err := p0.Close(); err != nil {
		t.Fatalf("unable to close failover Listener: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("failed to close the Portal: %v", err)
	}
	if err := p1.Close(); err != nil {
		t.Fatalf("unable to reclose \"abc\" Listener: %v", err)
	}
	if err := p2.Close(); err != nil {
		t.Fatalf("unable to close \"abd\" Listener: %v", err)
	}
	wg.Wait()

	t.Log("all done")
}
