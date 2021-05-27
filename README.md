# tcpsorter

## Overview

The package tcpsorter is an API to generate TCP net.Listener values
that can be differentiated by the first few bytes of the client
supplied byte stream data. This package can be used to support
multiple server instances on the same TCP port.

The support is similar in concept to a shell script executing the
`file -i` command and executing a different application with that file
as an argument. Supported services require that the TCP client send
data before the server, so the nature of the connection attempt can be
properly _sorted_.

## License info

The tcpsorter package is distributed with the same BSD 3-clause
license as that used by [golang](https://golang.org/LICENSE) itself.

## Reporting bugs and feature requests

Use the [github tcpsorter bug
tracker](https://github.com/tinkerator/tcpsorter/issues).
