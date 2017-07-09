package main

import (
	"fmt"
//	"io/ioutil"
//	"github.com/anivanovic/goTit/metainfo"
//	"crypto/sha1"
	"net"
	"encoding/binary"
	"strconv"
//	"bufio"
)

func CheckError(err error) {
    if err  != nil {
        fmt.Println("Error: " , err)
    }
}

func main() {
//	file, _ := ioutil.ReadFile("C:/Users/Antonije/Downloads/Allied (2016) [1080p] [YTS.AG].torrent")
//	fmt.Println("-------------------------------------------------------------------------------------")
//	_, printStr := metainfo.Decode(string(file), "", "")
//	fmt.Println(printStr)
//	
//	info, _ := ioutil.ReadFile("C:/Users/Antonije/Downloads/info.txt")
//	sha := sha1.New()
//	sha.Write([]byte(string(info)))
//	
//	hash := sha.Sum(nil)
//	fmt.Printf("info hash: %x\n", hash)
	
	udpAddr, err := net.ResolveUDPAddr("udp", "tracker.openbittorrent.com:80")
	CheckError(err)
	
	LocalAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
    CheckError(err)
    
    Conn, err := net.DialUDP("udp", LocalAddr, udpAddr)
    CheckError(err)
    
	defer Conn.Close()
	
	p :=  make([]byte, 2048)
	data := make([]byte, 16)
	action := uint32(0)
	
	connection_id, _ := strconv.ParseInt("41727101980", 16, 64)
	uconnection_id := uint64(connection_id)
	transaction_id := uint32(123986378)
	binary.BigEndian.PutUint64(data, uconnection_id)
	binary.BigEndian.PutUint32(data, action)
	binary.BigEndian.PutUint32(data, transaction_id)
	
	i, err := Conn.Write(data)
	fmt.Println("sent data to udp tracker: " + string(i))

	CheckError(err)
	
	red, err := Conn.Read(p)
	fmt.Println("read response")
	if red == 16 {
		fmt.Println("Read 16 bites")
	}
	fmt.Println("read " + string(red))
	fmt.Printf("%s\n", p)
}
