package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
)

type Service struct {
	Listen *net.TCPAddr
	Dial   *net.TCPAddr
}

func main() {

	debugLog := false
	debugLogString := os.Getenv("DEBUG_LOG")
	if regexp.MustCompile("1|t|y|true|yes").MatchString(strings.ToLower(debugLogString)) {
		debugLog = true
	}

	services := []Service{}
	for i := 0; i < 100; i++ {
		listen := os.Getenv(fmt.Sprintf("SERVICE_%d_LISTEN", i))
		dial := os.Getenv(fmt.Sprintf("SERVICE_%d_DIAL", i))
		if listen != "" && dial != "" {
			if strings.Contains(listen, "localhost") || strings.Contains(listen, "127.0.0.1") {
				log.Fatalf("\nSERVICE_%d_LISTEN: got '%s'. listening on localhost wont work, listen on all addresses like this: ':<port>'\n\n", i, listen)
			}
			listenAddr, err := net.ResolveTCPAddr("tcp", listen)
			if err != nil {
				log.Fatalf("\nSERVICE_%d_LISTEN: got '%s' expected tcp address. try like this: ':<port>'\n\n", i, listen)
			}
			dialAddr, err := net.ResolveTCPAddr("tcp", dial)
			if err != nil {
				log.Fatalf("\nSERVICE_%d_DIAL: got '%s' expected tcp address. try like this: '<hostname>:<port>'\n\n", i, listen)
			}

			testConnection, err := net.DialTCP("tcp", nil, dialAddr)
			if err != nil {
				log.Printf("WARNING: SERVICE_%d_DIAL: can't connect to '%s' right now", i, dial)
			}
			defer testConnection.Close()

			services = append(services, Service{
				Listen: listenAddr,
				Dial:   dialAddr,
			})
		}
	}

	for serviceId, service := range services {
		go (func() {
			listener, err := net.ListenTCP("tcp", service.Listen)
			if err != nil {
				panic(err)
			}
			log.Printf("Listening: %v\nProxying: %v\n\n", *service.Listen, *service.Dial)

			connectionId := 0
			for {
				clientConnection, err := listener.AcceptTCP()
				if err != nil {
					panic(err)
				}

				connectionId++
				myConnectionId := fmt.Sprintf("%d_%d", serviceId, connectionId)

				go (func(myConnectionId string, clientConnection *net.TCPConn) {
					defer clientConnection.Close()

					if debugLog {
						log.Printf(
							"start proxying connection %s: %s -> %s to %v",
							myConnectionId, clientConnection.RemoteAddr(), clientConnection.LocalAddr(), *service.Dial,
						)
					}

					proxyConnection, err := net.DialTCP("tcp", nil, service.Dial)
					if err != nil {
						log.Printf("ERROR: error occurred dailing %v: %s\n", *service.Dial, err)
						return
					}
					defer proxyConnection.Close()

					BlockingBidirectionalPipe(clientConnection, proxyConnection, "upstream", "downstream", myConnectionId, debugLog)

					if debugLog {
						log.Printf("done proxying connection %s", myConnectionId)
					}
				})(myConnectionId, clientConnection)

			}

		})()
	}
}

func BlockingBidirectionalPipe(conn1, conn2 net.Conn, name1, name2 string, connectionId string, debugLog bool) {
	chanFromConn := func(conn net.Conn) chan []byte {
		c := make(chan []byte)

		go func() {
			b := make([]byte, 1024)

			for {
				n, err := conn.Read(b)
				if n > 0 {
					res := make([]byte, n)
					// Copy the buffer so it doesn't get changed while read by the recipient.
					copy(res, b[:n])
					c <- res
				}
				if err != nil {
					c <- nil
					break
				}
			}
		}()

		return c
	}

	chan1 := chanFromConn(conn1)
	chan2 := chanFromConn(conn2)

	for {
		select {
		case b1 := <-chan1:
			if b1 == nil {
				if debugLog {
					log.Printf("connection %s %s EOF", connectionId, name1)
				}
				return
			} else {
				conn2.Write(b1)
			}
		case b2 := <-chan2:
			if b2 == nil {
				if debugLog {
					log.Printf("connection %s %s EOF", connectionId, name2)
				}
				return
			} else {
				conn1.Write(b2)
			}
		}
	}
}
