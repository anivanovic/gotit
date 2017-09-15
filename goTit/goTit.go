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
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

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
	file, _ := ioutil.ReadFile("C:/Users/Antonije/Downloads/Alien- Covenant (2017) [720p] [YTS.AG].torrent")
	fmt.Println("-------------------------------------------------------------------------------------")
	torrent := string(file)
	_, printStr := metainfo.Decode(torrent, "", "")
	fmt.Println(printStr)
	
	infoDict := torrent[strings.Index(torrent, "4:info")+6:len(torrent)-1]
	sha := sha1.New()
	sha.Write([]byte(string(infoDict)))
	
	var hash []byte
	hash = sha.Sum(nil)
	fmt.Printf("info hash: %x\n", hash)

	u, err := url.Parse("udp://p4p.arenabg.com:1337")
	CheckError(err)
	
	udpAddr, err := net.ResolveUDPAddr("udp", u.Host)
	fmt.Println("Connecting to. " + u.Host)
	CheckError(err)
    
    Conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 6679})
    CheckError(err)
    
//	defer Conn.Close()
	
	Conn.SetDeadline(time.Now().Add(time.Second * time.Duration(60)))
	
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
//	i, err := Conn.Write(request.Bytes())
//	fmt.Printf("sent data to udp tracker: %d \n", i)

	CheckError(err)
	
//	length, err := Conn.Read(p)
	fmt.Println("read response")
	if length == 16 {
		fmt.Println("Read 16 bites")
	}
	connVar := binary.BigEndian.Uint32(p[:4])
	transResp := binary.BigEndian.Uint32(p[4:8])
	connId := binary.BigEndian.Uint64(p[8:16])
	fmt.Println("rsponse: ", connVar, transResp, connId)
	
	request = new(bytes.Buffer)
	binary.Write(request, binary.BigEndian, connVar)
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
	fmt.Println("Send request")
	response := make([]byte, 0, 4096)
	tmp := make([]byte, 256)
	
	fmt.Println("reading")
	length, _, err = Conn.ReadFromUDP(tmp)
	response = append(response, tmp[:length]...)
	CheckError(err)
	fmt.Println("READ")
	fmt.Println("Dohvaćeno podataka ", len(response))
	
	resCode := binary.BigEndian.Uint32(response[:4])
	message := string(response[8:])
	fmt.Print("response code ", resCode, message)
}
