package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"

	"net"
	"net/url"
	"time"

	"github.com/anivanovic/goTit/metainfo"
)

const (
	letterBytes           = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	listenPort     uint16 = 8999
	DownloadFolder        = "C:/Users/Antonije/Downloads/"
)

func CheckError(err error) {
	if err != nil {
		log.Printf("%T %+v", err, err)
	}
}

func randStringBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return b
}

func readConn(conn net.Conn) []byte {
	response := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)

	for {
		conn.SetDeadline(time.Now().Add(timeout))
		n, err := conn.Read(tmp)
		if err != nil {
			CheckError(err)
			break
		}
		fmt.Println("Read data from ", n)
		response = append(response, tmp[:n]...)
	}

	return response
}

func readHandshake(conn net.Conn) []byte {
	response := make([]byte, 0, 68)

	conn.SetDeadline(time.Now().Add(timeout))
	n, err := conn.Read(response)
	if err != nil {
		CheckError(err)
		return response
	}
	fmt.Println("Read data from peer ", n)

	return response
}

func readResponse(response []byte) []peerMessage {
	read := len(response)
	currPossition := 0

	messages := make([]peerMessage, 0)
	for currPossition < read {
		size := int(binary.BigEndian.Uint32(response[currPossition : currPossition+4]))
		currPossition += 4
		fmt.Println("size", size)
		message := NewPeerMessage(response[currPossition : currPossition+size])
		fmt.Println("message type:", message.code)
		fmt.Printf("peer has the following peeces %b\n", message.payload)
		currPossition = currPossition + size
		messages = append(messages, *message)
	}

	return messages
}

func main() {
	torrentContent, _ := ioutil.ReadFile("C:/Users/Antonije/Downloads/Alien- Covenant (2017) [720p] [YTS.AG].torrent")
	fmt.Println("-------------------------------------------------------------------------------------")
	torrentString := string(torrentContent)
	_, benDict := metainfo.Parse(torrentString)
	fmt.Println(benDict.String())

	transactionId := uint32(12345612)
	peerId := randStringBytes(20)

	torrent := NewTorrent(*benDict)

	announceList := torrent.Announce_list
	announceList = append(announceList, torrent.Announce)
	ips := make(map[string]bool)
	for _, trackerUrl := range announceList {
		u, err := url.Parse(trackerUrl)
		CheckError(err)
		tracker_ips := announce(u, transactionId, torrent.Hash, peerId)
		for k, v := range *tracker_ips {
			ips[k] = v
		}
	}

	fmt.Println("peers size in pool", len(ips))

	for ip, _ := range ips {

		peer := Peer{ip, nil, torrent}
		connected := peer.Announce(peerId)
		if connected {
			go peer.GoMessaging()
		}
	}
}

func announce(u *url.URL, transactionId uint32, hash []byte, peerId []byte) *map[string]bool {
	tracker := Tracker(u)
	defer tracker.Close()
	connId := tracker.Handshake(transactionId)
	ips := tracker.Announce(connId, hash, transactionId, peerId)
	return ips
}
