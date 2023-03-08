package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	pb "github.com/khatna/moby-client/proto"
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// HTTP Protocol
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Wensocket route handler
func handler(w http.ResponseWriter, r *http.Request) {
	// Allow CORS
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// Dial gRPC server
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	grpcConnection, err := grpc.Dial(os.Getenv("GRPC_URL"), opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer grpcConnection.Close()
	client := pb.NewTxHandlerClient(grpcConnection)

	// main operation:
	// if we recieve a message, stop the current stream and make a new call

	clientLock := sync.Mutex{}
	var curCancel context.CancelFunc
	for {
		_, msg, err := conn.ReadMessage()

		if err != nil {
			log.Println(err)
			return
		}
		val, err := strconv.ParseFloat(string(msg), 32)
		if err != nil {
			log.Println(err)
			continue
		}

		if curCancel != nil {
			curCancel()
		}

		// cancel current gRPC call and create new context
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		curCancel = cancel

		stream, err := client.GetTransactions(ctx, &pb.Value{Value: float32(val)})
		if err != nil {
			log.Println(err)
			continue
		}

		// start sending
		go func() {
			for {
				tx, err := stream.Recv()
				if err != nil {
					log.Printf("client.GetTransactions failed: %v\n", err)
					return
				}
				clientLock.Lock()
				conn.WriteJSON(tx)
				clientLock.Unlock()
			}
		}()
	}
}

// Test route
func helloServer(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("This is an example server.\n"))
}

// start a server
func main() {
	http.HandleFunc("/", handler)
	http.HandleFunc("/hello", helloServer)

	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(os.Getenv("SSL_CN")),
		Cache:      autocert.DirCache("certs"),
	}

	server := &http.Server{
		Addr: ":" + os.Getenv("TLS_PORT"),
		TLSConfig: &tls.Config{
			GetCertificate: certManager.GetCertificate,
		},
	}

	go http.ListenAndServe(":80", certManager.HTTPHandler(nil))
	log.Fatal(server.ListenAndServeTLS("", ""))
}
