package socketclient

import (
	"sync"
	"fmt"
	"syscall"
)

const packageSize = 1024

type SocketClient struct {
	Fd 		  int
	RecvBuff  []byte
	recvLen   int
	recvguard sync.Mutex
	SendBuff  []byte
	sendLen   int
	sendguard sync.Mutex
}

func NewSocketClient(fd int) (*SocketClient){
	return &SocketClient{RecvBuff: make([]byte, packageSize * 1000), SendBuff: make([]byte, packageSize * 1000),
		Fd: fd}
}

func (this *SocketClient) Recv(buf []byte, len int) { // recv to buff
	this.recvguard.Lock()
	defer this.recvguard.Unlock()
	if this.recvLen + len > packageSize * 1000 {
		fmt.Print("not have enough size")
	} else {
		for _, b := range buf {
			this.RecvBuff = append(this.RecvBuff[:this.recvLen], b)
			this.recvLen += 1
		}
	}
	fmt.Printf("recvlen: %d\n", this.recvLen)
	fmt.Println(string(this.RecvBuff)) // do someting with recvbuf
}

func (this *SocketClient) Send(buf []byte, len int) (int) {
	nSend, err := syscall.Write(this.Fd, buf)
	if err != nil && (err == syscall.EAGAIN || err == syscall.EWOULDBLOCK) {
		this.sendguard.Lock()
		for _, b := range buf {
			this.SendBuff = append(this.SendBuff[:this.sendLen], b)
			this.sendLen += 1
		}
		//this.SendBuff[this.sendLen:] = buf
		//this.sendLen += len
		this.sendguard.Unlock()
		return nSend
	} else if err != nil && (err != syscall.EAGAIN || err != syscall.EWOULDBLOCK) {
		fmt.Printf("error: send: %v", err)
		return -1 // send failed
	} else { // send success
		fmt.Printf("send success: %d byte", nSend)
		return nSend
	}
}

func (this *SocketClient) DoSend() { // when network not busy again, use this func
	this.sendguard.Lock()
	defer this.sendguard.Unlock()
	if this.sendLen == 0 { // epollout
		return
	}
	nSend, err := syscall.Write(this.Fd, this.SendBuff)
	if err != nil && (err == syscall.EAGAIN || err == syscall.EWOULDBLOCK) {
		return
	} else if err != nil && (err != syscall.EAGAIN || err != syscall.EWOULDBLOCK) {
		fmt.Printf("error: send: %v", err)
	} else { // send success
		fmt.Printf("send success: %d byte", nSend)
		//this.SendBuff[0:] = this.SendBuff[nSend:this.sendLen]
		//this.sendLen -= nSend
		var nPos int;
		for _, b := range this.SendBuff[nSend:this.sendLen] {
			this.SendBuff = append(this.SendBuff[:nPos], b)
			nPos += 1
		}
		this.sendLen = nPos
	}
}