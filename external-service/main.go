package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	ServiceID int
	Listen    *net.TCPAddr
	Dial      *net.TCPAddr
	Dialer    *net.Dialer
	SNI       string
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
	servicesByPort := map[int][]Service{}
	for i := 0; i < serviceCount; i++ {
		listen := os.Getenv(fmt.Sprintf("SERVICE_%d_LISTEN", i))
		dial := os.Getenv(fmt.Sprintf("SERVICE_%d_DIAL", i))
		sni := os.Getenv(fmt.Sprintf("SERVICE_%d_SNI", i))
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
			if testConnection != nil {
				defer testConnection.Close()
			}

			if _, has := servicesByPort[listenAddr.Port]; !has {
				servicesByPort[listenAddr.Port] = []Service{}
			}
			if sni == "" {
				for _, service := range servicesByPort[listenAddr.Port] {
					if service.SNI == "" {
						log.Fatalf("\nSERVICE_%d: Found multiple services listening on port %d with no SNI (Server Name Indication). this is not allowed.\n\n", i, listenAddr.Port)
					}
				}
			}
			servicesByPort[listenAddr.Port] = append(servicesByPort[listenAddr.Port], Service{
				ServiceID: i,
				Listen:    listenAddr,
				Dial:      dialAddr,
				Dialer: &net.Dialer{
					Timeout:   dialTimeoutDuration,
					LocalAddr: dialFromAddr,
				},
				SNI: sni,
			})
		}
	}

	for port, services := range servicesByPort {
		port := port
		services := services
		go (func(port int, services []Service) {
			listener, err := net.ListenTCP("tcp", services[0].Listen)
			if err != nil {
				panic(err)
			}

			serviceBySNI := map[string]Service{}
			var defaultService Service

			for _, service := range services {
				if service.SNI == "" {
					defaultService = service
				} else {
					serviceBySNI[service.SNI] = service
				}
			}
			serviceStrings := []string{}
			for k, v := range serviceBySNI {
				serviceStrings = append(serviceStrings, fmt.Sprintf("sni='%s' -> %v", k, *v.Dial))
			}
			if defaultService.Dial != nil {
				serviceStrings = append(serviceStrings, fmt.Sprintf("<default> -> %v", *defaultService.Dial))
			}

			log.Printf("Listening: %v  Proxying: [%s]\n\n", *services[0].Listen, strings.Join(serviceStrings, ", "))

			connectionId := 0
			for {
				clientConnection, err := listener.AcceptTCP()
				if err != nil {
					panic(err)
				}

				connectionId++

				connectionHeader := make([]byte, 1024)
				n, err := clientConnection.Read(connectionHeader)
				if err != nil && err != io.EOF {
					log.Printf(
						"opening connection %d failed: TCP read error on %s -> %s when trying to scrape SNI header\n",
						connectionId, clientConnection.RemoteAddr(), clientConnection.LocalAddr(),
					)
					clientConnection.Close()
					continue
				}

				service := defaultService
				hostname, err := getHostnameFromSNI(connectionHeader[:n])
				if err != nil {
					var has bool
					service, has = serviceBySNI[hostname]
					if !has {
						service = defaultService
					}
				}

				myConnectionId := fmt.Sprintf("%d_%d", service.ServiceID, connectionId)

				go (func(myConnectionId string, connectionHeader []byte, service Service, clientConnection *net.TCPConn) {
					defer clientConnection.Close()
					if debugLog {
						log.Printf(
							"starting proxying connection %s: %s -> %s (%s) to %s\n",
							myConnectionId, clientConnection.RemoteAddr(), clientConnection.LocalAddr(), hostname, *service.Dial,
						)
					}
					proxyConnection, err := service.Dialer.Dial("tcp", service.Dial.String())
					if err != nil {
						log.Printf("ERROR: error occurred dailing %v: %s\n", *service.Dial, err)
						return
					} else if debugLog {
						log.Printf(
							"started proxying connection %s: %s -> %s (%s) to %s -> %v\n",
							myConnectionId, clientConnection.RemoteAddr(), clientConnection.LocalAddr(), hostname, proxyConnection.LocalAddr(), *service.Dial,
						)
					}
					defer proxyConnection.Close()

					_, err = proxyConnection.Write(connectionHeader)
					if err != nil {
						log.Printf("write error when writing header for connection %s, %s\n", myConnectionId, err)
						return
					}

					BlockingBidirectionalPipe(clientConnection, proxyConnection, "upstream", "downstream", myConnectionId, debugLog)

					if debugLog {
						log.Printf("done proxying connection %s\n", myConnectionId)
					}
				})(myConnectionId, connectionHeader, service, clientConnection)
			}
		})(port, services)
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
