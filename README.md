# gSoketManage
使用syscall包的原生epoll进行soketTCP编程

syscall.EPOLLET et模式  EPOLLET = -0x80000000是个负数，导致编译时错误

go build server.go 去除EPOLLET后没有任何反应，没错误，也没文件生成。。。原因是package main的main单词写错了。。
