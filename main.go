package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
)

const BlockSize = 512

var conn *net.UDPConn
var tids = make(map[[6]byte]*os.File)

func sendError(code byte, peer *net.UDPAddr) {
	conn.WriteToUDP([]byte{0, 5, 0, code, 0}, peer)
}

func addr2Tid(peer *net.UDPAddr) [6]byte {
	var tid []byte
	tid = append(tid, peer.IP...)
	tid = append(tid[4:], byte(peer.Port>>8), byte(peer.Port&0xff))
	return [6]byte(tid)
}

func sendBlock(file *os.File, block int64, peer *net.UDPAddr) {
	buffer := make([]byte, BlockSize)
	n, err := file.Read(buffer)
	if err != nil {
		/* Clear TID if we already sent the last block */
		if errors.Is(err, io.EOF) {
			delete(tids, addr2Tid(peer))
			file.Close()
			return
		}
		fmt.Println("[ERROR] Cannot read file")
		sendError(2, peer)
		return
	}

	if n != BlockSize {
		buffer = buffer[:n]
	}

	response := []byte{0, 3, 0, 0}
	binary.BigEndian.PutUint16(response[2:], uint16(block))

	conn.WriteToUDP(append(response, buffer...), peer)
}

func main() {
	var err error
	addr, _ := net.ResolveUDPAddr("udp", "0.0.0.0:69")
	conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("[FATAL] Cannot listen on 0.0.0.0:69")
		os.Exit(1)
	}

	fmt.Println("Server stared at 0.0.0.0:69")

	for {
		var n int
		var peer *net.UDPAddr
		buffer := make([]byte, 512)
		n, peer, err = conn.ReadFromUDP(buffer)
		if err != nil || n < 4 || (buffer[1] != 1 && buffer[1] != 4) {
			fmt.Println("[ERROR] Invalid UDP packet from", peer, buffer)
			sendError(4, peer)
			continue
		}
		buffer = buffer[:n]

		if buffer[1] == 1 {
			/* Packet is an initial read request */
			var i int
			for i = 2; i < len(buffer) && buffer[i] != 0; i++ {
			}
			fileName := string(buffer[2:i])

			/* Only support file in the same directories where the server is started */
			if i == len(buffer) || path.Base(fileName) != fileName {
				fmt.Println("[WARNING] Invalid file name from", peer, fileName)
				sendError(4, peer)
				continue
			}

			/* Open file and add TID */
			var file *os.File
			file, err = os.Open(fileName)
			if err != nil {
				fmt.Println("[ERROR] File not found:", fileName)
				sendError(1, peer)
				continue
			}

			tids[addr2Tid(peer)] = file

			go sendBlock(file, 1, peer)
		} else if buffer[1] == 4 {
			/* Packet is an acknowledgment */
			file, ok := tids[addr2Tid(peer)]
			if !ok {
				fmt.Println("Unknown TID from", peer)
				continue
			}

			go sendBlock(file, int64(binary.BigEndian.Uint16(buffer[2:4]))+1, peer)
		}
	}
}
