package gotit

import (
	"context"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
)

func readConn(ctx context.Context, conn net.Conn) []byte {
	response := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)

	for {
		conn.SetDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(tmp)
		if err != nil {
			CheckError(err)
			break
		}
		response = append(response, tmp[:n]...)
	}

	return response
}

func CheckError(err error) {
	if err != nil {
		log.Warnf("%T %+v", err, err)
	}
}
