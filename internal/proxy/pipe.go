package proxy

import (
	"io"
	"net"
)

// Pipe copia nos dois sentidos até ambos terminarem.
func Pipe(client net.Conn, clientR io.Reader, up net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(up, clientR)
		closeWrite(up)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, up)
		closeWrite(client)
		done <- struct{}{}
	}()
	<-done
	<-done
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	} else {
		_ = c.Close()
	}
}
