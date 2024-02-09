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

func sendBlock(file *os.File, block uint16, peer *net.UDPAddr) {
	response := []byte{0, 3, 0, 0}
	binary.BigEndian.PutUint16(response[2:], block)
	buffer := make([]byte, BlockSize)
	file.Seek(int64(block-1)*BlockSize, io.SeekStart)
	n, err := file.Read(buffer)

	if err != nil {
		/* File size is multiple of block size, reply an empty data packet. */
		if errors.Is(err, io.EOF) {
			conn.WriteToUDP(response, peer)
			delete(tids, addr2Tid(peer))
			file.Close()
			return
		}
		fmt.Println("[ERROR] Cannot read file")
		sendError(2, peer)
		return
	}

	/**
	 * File size is NOT multiple of block size.
	 * The client should know there's no more packet to send.
	 */
	if n != BlockSize {
		buffer = buffer[:n]
		delete(tids, addr2Tid(peer))
		file.Close()
	}

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

			sendBlock(file, 1, peer)
		} else if buffer[1] == 4 {
			/* Packet is an acknowledgment */
			file, ok := tids[addr2Tid(peer)]
			if !ok {
				fmt.Println("Unknown TID from", peer)
				continue
			}

			sendBlock(file, binary.BigEndian.Uint16(buffer[2:4])+1, peer)
		}
	}
}
