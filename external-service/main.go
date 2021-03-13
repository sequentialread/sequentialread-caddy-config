package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	Listen  *net.TCPAddr
	Dial    *net.TCPAddr
	Timeout time.Duration
}

func main() {

	log.Println("")

	debugLog := false
	debugLogString := os.Getenv("DEBUG_LOG")
	if regexp.MustCompile("1|t|y|true|yes").MatchString(strings.ToLower(debugLogString)) {
		debugLog = true
	}

	serviceCount := 10
	serviceCountString := os.Getenv("SERVICE_COUNT")
	if serviceCountString != "" {
		var err error
		serviceCount, err = strconv.Atoi(serviceCountString)
		if err != nil {
			log.Fatalf("\nSERVICE_COUNT value '%s' must be an integer\n\n", serviceCountString)
		}
	}

	var dialFromAddr *net.TCPAddr
	dialFromString := os.Getenv("DIAL_FROM")
	if serviceCountString != "" {
		var err error
		dialFromAddr, err = net.ResolveTCPAddr("tcp", dialFromString)
		if err != nil {
			log.Fatalf("DIAL_FROM value '%s' must be a TCP address. Try '<ip_address>:'\n\n", dialFromString)
		}
	}
	services := []Service{}
	for i := 0; i < serviceCount; i++ {
		listen := os.Getenv(fmt.Sprintf("SERVICE_%d_LISTEN", i))
		dial := os.Getenv(fmt.Sprintf("SERVICE_%d_DIAL", i))
		dialTimeout := os.Getenv(fmt.Sprintf("SERVICE_%d_DIAL_TIMEOUT", i))
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
			dialTimeoutDuration := time.Second * time.Duration(5)
			if dialTimeout != "" {
				dialTimeoutDuration, err = time.ParseDuration(dialTimeout)
				if err != nil {
					log.Fatalf("\nSERVICE_%d_DIAL_TIMEOUT: got '%s' expected duration. try like this: '5s'\n\n", i, dialTimeout)
				}
			}

			testConnection, err := net.DialTCP("tcp", nil, dialAddr)
			if err != nil {
				log.Printf("WARNING: SERVICE_%d_DIAL: can't connect to '%s' right now", i, dial)
			}
			defer testConnection.Close()

			services = append(services, Service{
				Listen:  listenAddr,
				Dial:    dialAddr,
				Timeout: dialTimeoutDuration,
			})
		}
	}

	for serviceId, service := range services {
		service := service
		serviceId := serviceId
		go (func(serviceId int, service *Service) {
			listener, err := net.ListenTCP("tcp", service.Listen)
			dialer := net.Dialer{
				Timeout:   service.Timeout,
				LocalAddr: dialFromAddr,
			}
			if err != nil {
				panic(err)
			}
			log.Printf("Listening: %v  Proxying: %v\n\n", *service.Listen, *service.Dial)

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
							"starting proxying connection %s: %s -> %s to %s\n",
							myConnectionId, clientConnection.RemoteAddr(), clientConnection.LocalAddr(), *service.Dial,
						)
					}
					proxyConnection, err := dialer.Dial("tcp", service.Dial.String())
					if err != nil {
						log.Printf("ERROR: error occurred dailing %v: %s\n", *service.Dial, err)
						return
					} else if debugLog {
						log.Printf(
							"started proxying connection %s: %s -> %s to %s -> %v\n",
							myConnectionId, clientConnection.RemoteAddr(), clientConnection.LocalAddr(), proxyConnection.LocalAddr(), *service.Dial,
						)
					}
					defer proxyConnection.Close()

					BlockingBidirectionalPipe(clientConnection, proxyConnection, "upstream", "downstream", myConnectionId, debugLog)

					if debugLog {
						log.Printf("done proxying connection %s\n", myConnectionId)
					}
				})(myConnectionId, clientConnection)
			}
		})(serviceId, &service)
	}

	// block forever
	select {}
}

func BlockingBidirectionalPipe(conn1, conn2 net.Conn, name1, name2 string, connectionId string, debugLog bool) {
	chanFromConn := func(conn net.Conn, name, connectionId string) chan []byte {
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
					log.Printf("%s %s read error %s\n", connectionId, name, err)
					c <- nil
					break
				}
			}
		}()

		return c
	}

	chan1 := chanFromConn(conn1, fmt.Sprint(name1, "->", name2), connectionId)
	chan2 := chanFromConn(conn2, fmt.Sprint(name2, "->", name1), connectionId)

	for {
		select {
		case b1 := <-chan1:
			if b1 == nil {
				if debugLog {
					log.Printf("connection %s %s EOF\n", connectionId, name1)
				}
				return
			} else {
				conn2.Write(b1)
			}
		case b2 := <-chan2:
			if b2 == nil {
				if debugLog {
					log.Printf("connection %s %s EOF\n", connectionId, name2)
				}
				return
			} else {
				conn1.Write(b2)
			}
		}
	}
}
