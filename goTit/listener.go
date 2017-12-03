package main

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

//func main() {
//	listen(10001)
//}

func listen(port int) {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(port))

	if err != nil {
		CheckError(err)
		return
	}
	fmt.Println(listener.Addr())

	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			CheckError(err)
			continue
		}

		go handlePeerConnection(conn)
	}
}

func handlePeerConnection(conn net.Conn) {
	response := readConn(conn)
	checkHandshake(response, nil, nil)

	conn.SetDeadline(time.Now().Add(time.Second * 5))
	conn.Write(response[0:68])
	//conn.Write(createBitfieldMessage())
	fmt.Println("send handhake and bitfield")

	if len(response) > 68 {
		peerMessages := readResponse(response[68:])
		handlePeerMessages(peerMessages, conn)
	} else {
		fmt.Println("additional reading from peer")
		peerMessages := readResponse(readConn(conn))
		handlePeerMessages(peerMessages, conn)
	}

}

func handlePeerMessages(messages []peerMessage, conn net.Conn) {

	for _, message := range messages {
		switch message.code {
		case notInterested:
			fmt.Println("notInterested")
		case interested:
			fmt.Println("interested")
			conn.SetDeadline(time.Now().Add(time.Second * 5))
			conn.Write(createUnchokeMessage())
		case unchoke:
			fmt.Println("unchoke")
		case choke:
			fmt.Println("choke")
		case bitfield:
			fmt.Printf("bitfield %b\n", message.payload)
		default:
			fmt.Println("Got", message.code, "code")
		}
	}
}
