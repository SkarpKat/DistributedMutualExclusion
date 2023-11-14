package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	pb "github.com/SkarpKat/DistributedMutualExclusion/proto"
	"google.golang.org/grpc"
)

var (
	id                  = flag.Int("id", 0, "id of the node")
	nodeTimestamp int64 = 0
	state               = "RELEASED"
	portsPath           = "ports.txt"
	clients             = make(map[int32]pb.ServiceClient)
)

type NodeServiceServer struct {
	pb.UnimplementedServiceServer
}

func updateLamportTimestamp(timestamp int64) {
	if timestamp > nodeTimestamp {
		nodeTimestamp = timestamp + 1
	} else {
		nodeTimestamp++
	}
}

func (n *NodeServiceServer) Request(ctx context.Context, in *pb.RequestMessage) (*pb.ResponseMessage, error) {

	for state == "HELD" {
		time.Sleep(1 * time.Second)
	}

	for state == "WANTED" && in.Timestamp > nodeTimestamp {
		time.Sleep(1 * time.Second)
	}

	updateLamportTimestamp(in.Timestamp)
	return &pb.ResponseMessage{Timestamp: nodeTimestamp, Message: "OK", Id: int32(*id)}, nil

}

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logPath := "logs/node" + strconv.Itoa(*id) + ".log"
	sharedFilePath := "sharedFile.log"

	// WRONLY = Write Only, 0644 = file permission that allows read and write for owner, and read for everyone else
	file, err := os.OpenFile(logPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	log.SetOutput(io.MultiWriter(file, os.Stdout))

	portFile, err := os.Open(portsPath)
	if err != nil {
		log.Fatal(err)
	}

	ports := make([]string, 0)
	scanner := bufio.NewScanner(portFile)
	for scanner.Scan() {
		log.Printf("Port: %v added to slice", scanner.Text())
		ports = append(ports, scanner.Text())
	}

	var port string

	for i := 0; i < len(ports); i++ {
		if strings.Split(ports[i], ",")[0] == strconv.Itoa(*id) {
			port = strings.Split(ports[i], ",")[1]
		}
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%v", port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Make node host a server
	grpcServer := grpc.NewServer()

	pb.RegisterServiceServer(grpcServer, &NodeServiceServer{})

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	for i := 0; i < len(ports); i++ {
		if strings.Split(ports[i], ",")[0] == strconv.Itoa(*id) {
			continue
		}
		port := strings.Split(ports[i], ",")[1]
		log.Printf("Dialing port %v", port)
		conn, err := grpc.Dial(fmt.Sprintf(":%v", port), grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Fatalf("did not connect: %v", err)
		}
		defer conn.Close()
		c := pb.NewServiceClient(conn)
		clients[int32(i)] = c
	}

	// Make command line interface
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for {
			for scanner.Scan() {
				command := scanner.Text()
				switch command {
				case "request":
					if state == "RELEASED" {
						state = "WANTED"
						for _, client := range clients {
							client.Request(ctx, &pb.RequestMessage{Id: int32(*id), Timestamp: nodeTimestamp, Message: "WANTED"})
						}
						state = "HELD"
						log.Printf("Node %v is in critical section", *id)
						file, err := os.OpenFile(sharedFilePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
						if err != nil {
							log.Fatal(err)
						}
						file.WriteString("Node " + strconv.Itoa(*id) + " is in critical section\n")
						criticalScanner := bufio.NewScanner(os.Stdin)
						for criticalScanner.Scan() {
							if criticalScanner.Text() == "exit" {
								break
							}
							file.WriteString(criticalScanner.Text() + "\n")
						}
						file.Close()
						state = "RELEASED"
						log.Printf("Node %v is out of critical section", *id)
					} else {
						log.Printf("Node %v is already in critical section", *id)
					}
				case "exit":
					os.Exit(0)
				}
			}
		}
	}()

	// Start listening for requests
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

}
