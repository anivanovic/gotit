package main

import (
	"fmt"
	"bytes"
	"strings"
	"io/ioutil"
	"github.com/anivanovic/goTit/metainfo"
	"crypto/sha1"
	"net"
	"net/url"
	"encoding/binary"
	"time"
	"math/rand"
	"io"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const BITTORENT_PROT = "BitTorrent protocol"

func CheckError(err error) {
	if err  != nil {
		fmt.Println("Error: " , err)
	}
}

func randStringBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return b
}

func main() {
	file, _ := ioutil.ReadFile("C:/Users/eaneivc/Downloads/Wonder Woman (2017) [720p] [YTS.AG].torrent")
	fmt.Println("-------------------------------------------------------------------------------------")
	torrent := string(file)
	_, benDict := metainfo.Decode(torrent)
	fmt.Println(benDict.GetData())
	
	infoDict := torrent[strings.Index(torrent, "4:info")+6:len(torrent)-1]
	sha := sha1.New()
	sha.Write([]byte(string(infoDict)))
	
	var hash []byte
	hash = sha.Sum(nil)
	fmt.Printf("info hash: %x\n", hash)

	u, err := url.Parse("udp://tracker.coppersurfer.tk:6969/announce")
	CheckError(err)
	
	udpAddr, err := net.ResolveUDPAddr("udp", u.Host)
	fmt.Println("Connecting to: " + u.Host)
	CheckError(err)

	Conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 6679})
	CheckError(err)
	
	defer Conn.Close()
	
	Conn.SetDeadline(time.Now().Add(time.Second * time.Duration(5)))
	
	request := new(bytes.Buffer)
	p := make([]byte, 16)
	
	var action uint32 = 0
	var connection_id uint64 = 0x41727101980
	transaction_id := uint32(12398636)
	
	binary.Write(request, binary.BigEndian, connection_id)
	binary.Write(request, binary.BigEndian, action)
	binary.Write(request, binary.BigEndian, transaction_id)
	
	Conn.WriteTo(request.Bytes(), udpAddr)
	length, _, err := Conn.ReadFromUDP(p)

	CheckError(err)
	
	fmt.Println("read response")
	if length == 16 {
		fmt.Println("Read 16 bites")
	}
	connVar := binary.BigEndian.Uint32(p[:4])
	transResp := binary.BigEndian.Uint32(p[4:8])
	connId := binary.BigEndian.Uint64(p[8:16])
	fmt.Println("rsponse: ", connVar, transResp, connId)
	
	request = new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, connId)
	binary.Write(request, binary.BigEndian, uint32(1))
	binary.Write(request, binary.BigEndian, uint32(127545))
	binary.Write(request, binary.BigEndian, hash)
	peer_id := randStringBytes(20)
	binary.Write(request, binary.BigEndian, peer_id)
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, uint64(960989559))
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, uint32(0))
	binary.Write(request, binary.BigEndian, uint32(0))
	randKey := rand.Int31()
	binary.Write(request, binary.BigEndian, randKey)
	binary.Write(request, binary.BigEndian, int32(-1))
	binary.Write(request, binary.BigEndian, uint16(6679))
	
	Conn.WriteTo(request.Bytes(), udpAddr)
	fmt.Println("Send announce")
	response := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	
	fmt.Println("reading")
	
	for {
	n, err := Conn.Read(tmp)
		if err != nil {
			if err != io.EOF {
				CheckError(err)
			}
			break
		}
		response = append(response, tmp[:n]...)

	}
	fmt.Println("READ")
	fmt.Println("Dohvaćeno podataka ", len(response))
	
	resCode := binary.BigEndian.Uint32(response[:4])
	transaction_id = binary.BigEndian.Uint32(response[4:8])
	interval := binary.BigEndian.Uint32(response[8:12])
	leachers := binary.BigEndian.Uint32(response[12:16])
	seaders := binary.BigEndian.Uint32(response[16:20])
	peerCount := (len(response) -20) / 6
	peerAddresses := response[20:]
	ports := make([]uint16, 0)
	ips := make([]string, 0)
	fmt.Println("Peer count ", peerCount)
	for i := 0; i < peerCount; i++ {
		byteMask := 6
		ipAddress := fmt.Sprintf(" %d.%d.%d.%d ", peerAddresses[byteMask * i], response[byteMask * i + 1], response[byteMask * i + 2], response[byteMask * i + 3])
		port := binary.BigEndian.Uint16(response[byteMask * i + 4:byteMask * i + 6])
		ports = append(ports, port)
		ips = append(ips, ipAddress)
		fmt.Println("response code ", resCode, transaction_id, interval, leachers, seaders, ipAddress, port)
	}
	
	fmt.Println(ports)
	fmt.Println(ips)
	
	
	request = new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, 19)
	binary.Write(request, binary.BigEndian, BITTORENT_PROT)
	binary.Write(request, binary.BigEndian, uint64(0))
	binary.Write(request, binary.BigEndian, hash)
	binary.Write(request, binary.BigEndian, randStringBytes(20))
	
	Conn.Write(request.Bytes())
	
}