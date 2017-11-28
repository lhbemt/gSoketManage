package socketmanage

import (
	"syscall"
	"sync"
	"fmt"
	"strings"
	"strconv"
	"unsafe"
	"../socketclient"
)

const (
	epollEnvCount = 100000
	packageSize = 1024
)

type socketRecvPackage struct {
	fd  int
	buf []byte
	len int
}

type socketSendPackage struct {
	fd  int
	len int
}

type SocketServerManage struct {
	serveraddr 	 string
	serversocket int
	clients      map[int](bool)
	clisockets   map[int](*socketclient.SocketClient)
	clientsguard sync.Mutex
	start        bool
	listensocket int
	epollfd      int
	recvBytes    chan socketRecvPackage
	ready        chan byte
	stop         chan byte // ternimate routine
}

type _Socklen uint32

type sockaddr4 struct {
	sa syscall.SockaddrInet4
	syscall.Sockaddr // need write interface first
}

func (this sockaddr4) sockaddr() (unsafe.Pointer, _Socklen, error) {
	return unsafe.Pointer(&this.sa), _Socklen(unsafe.Sizeof(this.sa)), nil
}

func inet_addr(ipaddr string) ([4]byte, error) {
	var (
		ips = strings.Split(ipaddr, ".")
		ip  [4]uint64
		ret [4]byte
	)

	if len(ips) < 4 {
		return ret, fmt.Errorf("parse addr: %s error", ipaddr)
	}

	for i := 0; i < 4; i++ {
		ip[i], _ = strconv.ParseUint(ips[i], 10, 8)
	}
	for i := 0; i < 4; i++ {
		ret[i] = byte(ip[i])
	}
	return ret, nil
}

func checkBigEnd() bool {
	var t uint32
	t = 0x1234
	p := unsafe.Pointer(&t)
	p2 := (*[2]byte)(p)
	if p2[0] == 52 { // 0x34
		return false
	} else { // 0x12
		return true
	}
}

func htons(port uint16) uint16 {
	if checkBigEnd() {
		return port
	} else {
		var hport uint16
		hport = port << 8 + port >> 8
		return hport
	}
}

func (this *SocketServerManage) epollInit() error {
	var err error
	this.epollfd, err = syscall.EpollCreate(5)
	if err != nil {
		return fmt.Errorf("epollcreate error: %v", err)
	}
	return nil
}

func (this *SocketServerManage) epollAttach(fd int) error {
	var err error
	var epollenv syscall.EpollEvent
	epollenv.Fd = int32(fd)
	epollenv.Events = syscall.EPOLLIN|syscall.EPOLLOUT // ETmode
	err = syscall.EpollCtl(this.epollfd, syscall.EPOLL_CTL_ADD, fd, &epollenv)
	if err != nil {
		return fmt.Errorf("epollctl error: %v", err)
	}
	return nil
}

func (this *SocketServerManage) init(addr string, port int) (bool, error) {
	if this.start {
		return false, fmt.Errorf("error: already running")
	}

	var err error
	this.listensocket, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return false, fmt.Errorf("error: listen: %v", err)
	}

	err = this.epollInit()
	if err != nil {
		return false, fmt.Errorf("error: %v", err)
	}

	err = syscall.SetNonblock(this.listensocket, true)
	if err != nil {
		return false, fmt.Errorf("error: setNonblock: %v", err)
	}

	var sa syscall.SockaddrInet4
	sa.Addr, err = inet_addr(addr)
	if err != nil {
		return false, fmt.Errorf("error: %v", err)
	}
	sa.Port = int(htons(uint16(port)))
	err = syscall.Bind(this.listensocket, &sa)
	if err != nil {
		return false, fmt.Errorf("error: bind: %v", err)
	}

	fmt.Printf("listen: %s, port: %d", sa.Addr, sa.Port)

	err = syscall.Listen(this.listensocket, syscall.SOMAXCONN)
	if err != nil {
		return false, fmt.Errorf("error: listen: %v", err)
	}

	err = this.epollAttach(this.listensocket)
	if err != nil {
		return false, fmt.Errorf("error: epollAttach: %v", err)
	}

	return true, nil
}

func (this *SocketServerManage) epollWork() {

	var epollenvs []syscall.EpollEvent
	epollenvs = make([]syscall.EpollEvent, epollEnvCount)

	for {
		select {
		case <-this.ready :
			ready, err := syscall.EpollWait(this.epollfd, epollenvs, -1)
			if err != nil && err != syscall.EAGAIN {
				fmt.Printf("epoll_wait error: %v\n", err)
				return
			} else if err != nil && (err == syscall.EAGAIN || err == syscall.EINTR) {
				continue
			} else {
				for i := 0; i < ready; i++ {
					if epollenvs[i].Fd == int32(this.listensocket) { // listen
						this.doAccept(epollenvs[i])
					} else {
						this.doSocket(epollenvs[i])
					}
				}
			}
			this.ready <- '0'
		case <-this.stop :
			return //ternimate routine
		}
	}
}

func (this *SocketServerManage) doAccept(env syscall.EpollEvent) {
	for {
		fd, _, err := syscall.Accept(int(env.Fd))
		if err != nil && err != syscall.EAGAIN {
			fmt.Printf("accept error: %v", err)
			return
		} else if err != nil && err == syscall.EAGAIN {
			return
		} else { // new client
			this.clientsguard.Lock()
			client := socketclient.NewSocketClient(fd)
			this.clients[fd] = true
			this.clisockets[fd] = client
			this.clientsguard.Unlock()
			this.epollAttach(fd) //attach
			continue
		}
	}
}

func (this *SocketServerManage) doSocket(env syscall.EpollEvent) {
	iRet := env.Events & syscall.EPOLLIN
	if iRet == 1 {
		this.doRead(env)
	} else {
		this.doSend(env)
	}
}

func (this *SocketServerManage) doRead(env syscall.EpollEvent) {
	for {
		buf := make([]byte, packageSize)
		nlen, err := syscall.Read(int(env.Fd), buf)
		if err != nil && (err != syscall.EAGAIN || err != syscall.EWOULDBLOCK) {
			fmt.Printf("read error: %v", err)
			return
		} else if err != nil && (err == syscall.EAGAIN || err == syscall.EWOULDBLOCK) {
			return
		} else {
			var recv socketRecvPackage
			recv.buf = buf[0:]
			recv.len = nlen
			recv.fd = int(env.Fd)
			this.recvBytes <- recv
			continue
		}
	}
}

func (this *SocketServerManage) doSend(env syscall.EpollEvent) {
	sock, _ := this.clisockets[int(env.Fd)]
	sock.DoSend()
}

func (this *SocketServerManage) cleanSocket(fd int) {
	this.clientsguard.Lock()
	syscall.Close(fd)
	delete(this.clients, fd)
	delete(this.clisockets, fd)
	this.clientsguard.Unlock()
	syscall.EpollCtl(this.epollfd, syscall.EPOLL_CTL_DEL, fd, nil)
}

func (this *SocketServerManage) withRecv() {
	for recv := range this.recvBytes {
		_, ok := this.clisockets[recv.fd]
		if !ok {
			continue
		} else {
			if recv.len == 0 { // socket close
				this.cleanSocket(recv.fd)
			}
			if this.clisockets[recv.fd] != nil {
				// socket operate
				go this.clisockets[recv.fd].Recv(recv.buf, recv.len)
			}
		}
	}
}

func New() *SocketServerManage {
	sock := &SocketServerManage{
		clients: make(map[int]bool),
		clisockets: make(map[int]*socketclient.SocketClient),
		recvBytes: make(chan socketRecvPackage),
		ready: make(chan byte),
		stop: make (chan byte),
	}

	return sock
}

func (this *SocketServerManage) Work() error {
	if this.start {
		return fmt.Errorf("already running")
	}

	_, err := this.init("127.0.0.1", 8080)
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	this.start = true

	go this.epollWork()
	go this.withRecv()

	this.ready <- '0'

	return nil
}

func (this *SocketServerManage) Stop() {
	close(this.ready)
	close(this.stop)
	close(this.recvBytes)
	for fd, _ := range this.clisockets {
		this.cleanSocket(fd)
	}
	syscall.Close(this.epollfd)
}
