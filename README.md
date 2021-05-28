# tcpsorter

## Overview

The package tcpsorter is an API to generate TCP net.Listener values
that can be differentiated by the first few bytes of the client
supplied byte stream data. This package can be used to support
multiple server instances on the same TCP port.

The support is similar in concept to a shell script executing the
`file -i` command and based on the determined mime-type, executing a
different application with that file as an argument. Supported
services require that the TCP client send data before the server, so
the nature of the connection attempt can be properly _sorted_.

Automated documentation for this Go package is available from
[pkg.go.dev](https://pkg.go.dev/zappem.net/pub/net/tcpsorter).

## What can this package do?

If you are curious how network protocols actually work, here is a
technique for watching their startup negotiation:
```
$ nc -l 8080 | xxd -g1
```
What this does is _listen_ to TCP port `8080` and create an `xxd` byte
dump of the first attempted incoming connection's data.

Note: in the various examples below, you will need to re-run the above
command because it will exit when the first connection is terminated.

For a first example (from a different terminal) try this:
```
$ wget localhost:8080
<Ctrl-C>
```
This will cause this `nc/xxd` pipeline to dump something like the
following to the terminal:
```
00000000: 47 45 54 20 2f 20 48 54 54 50 2f 31 2e 31 0d 0a  GET / HTTP/1.1..
00000010: 55 73 65 72 2d 41 67 65 6e 74 3a 20 57 67 65 74  User-Agent: Wget
00000020: 2f 31 2e 32 31 2e 31 0d 0a 41 63 63 65 70 74 3a  /1.21.1..Accept:
00000030: 20 2a 2f 2a 0d 0a 41 63 63 65 70 74 2d 45 6e 63   */*..Accept-Enc
00000040: 6f 64 69 6e 67 3a 20 69 64 65 6e 74 69 74 79 0d  oding: identity.
00000050: 0a 48 6f 73 74 3a 20 6c 6f 63 61 6c 68 6f 73 74  .Host: localhost
00000060: 3a 38 30 38 30 0d 0a 43 6f 6e 6e 65 63 74 69 6f  :8080..Connectio
00000070: 6e 3a 20 4b 65 65 70 2d 41 6c 69 76 65 0d 0a 0d  n: Keep-Alive...
```

Contrast this with an `ssh` client connection attempt:
```
$ ssh -p8080 localhost
<Ctrl-C>
```
which will dump:
```
00000000: 53 53 48 2d 32 2e 30 2d 4f 70 65 6e 53 53 48 5f  SSH-2.0-OpenSSH_
00000010: 38 2e 35 0d 0a                                   8.5..
```

Another example:
```
$ telnet localhost 8080
Trying ::1...
Connected to localhost.
Escape character is '^]'.
abc
<Ctrl-]>
telnet> c
Connection closed.
```
(Here, you type `abc<ENTER><Ctrl-]>c<ENTER>` to complete the connection.)

This causes the following to be dumped:
```
00000000: 61 62 63 0d 0a                                   abc..
```
That is, the telnet protocol simply creates a connection and assumes
the server is a telnet server - there is no client initiated protocol
negotiation.

With this in mind, you can use
[`"zappem.net/pub/net/tcpsorter"`](https://pkg.go.dev/zappem.net/pub/net/tcpsorter)
to set up different protocol servers all attached to the same port via
a `tcpsorter.NewPortal()` and `(*tcpsorter.Portal).Listen()` calls for
each different protocol. In this case, we would assign the telnet protocol as the default listener. Something like this:
```
portal, _ := tcpsorter.NewPortal(":8080")
h, _ := portal.Listen([]byte("GET "), []byte("PUT "), []byte("POST "), []byte("PATCH "), []byte("DELETE "))
ssh, _ := portal.Listen([]byte("SSH-2.0-OpenSSH"))
telnet, _ := portal.Listen()
go portal.Run(10*time.Second)
```

Of course, you will need to write the code to connect the http server
to this `h` `net.Listener` interface etc, but this is a sketch of how
one can use the `tcpsorter` package to multiplex different protocols
to a single port.

Note: protocols that initiate with the _server_ sending data will
conflict with the `telnet` connection in this example, and any server
using `tcpsorter` can pick only one such server backend to be the
default listener.

## License info

The tcpsorter package is distributed with the same BSD 3-clause
license as that used by [golang](https://golang.org/LICENSE) itself.

## Reporting bugs and feature requests

Use the [github tcpsorter bug
tracker](https://github.com/tinkerator/tcpsorter/issues).
