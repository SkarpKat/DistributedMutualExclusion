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

	pb "github.com/SkarpKat/DistributedMutualExclusion/proto"
	"google.golang.org/grpc"
)

var (
	id                  = flag.Int("id", 0, "id of the node")
	nodeTimestamp int64 = 0
	state               = "RELEASED"
	portsPath           = "ports.txt"
	queue               = make([]int32, 0)
)

type NodeServiceServer struct {
	pb.UnimplementedServiceServer
	pb.RequestMessage
	pb.ResponseMessage
}

func updateLamportTimestamp(timestamp int64) {
	if timestamp > nodeTimestamp {
		nodeTimestamp = timestamp + 1
	} else {
		nodeTimestamp++
	}
}

func (n *NodeServiceServer) Request(ctx context.Context, in *pb.RequestMessage) (*pb.ResponseMessage, error) {
	// implementation goes here
	updateLamportTimestamp(in.GetTimestamp())
	// Implicit call to request service defined in proto file
	log.Printf("Received request from node %v with timestamp %v", in.GetId(), in.GetTimestamp())
	if state == "HELD" || (state == "WANTED" && in.GetTimestamp() < nodeTimestamp) {
		queue = append(queue, in.GetId())
		log.Printf("Node %v added to queue", in.GetId())
		updateLamportTimestamp(in.GetTimestamp())
		return &pb.ResponseMessage{Message: state, Timestamp: nodeTimestamp}, nil
	} else {
		updateLamportTimestamp(in.GetTimestamp())
		return &pb.ResponseMessage{Message: state, Timestamp: nodeTimestamp}, nil
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flag.Parse()

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

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))

	// Make node host a server
	grpcServer := grpc.NewServer()

	Listener := &NodeServiceServer{
		UnimplementedServiceServer: pb.UnimplementedServiceServer{},
		RequestMessage:             pb.RequestMessage{},
		ResponseMessage:            pb.ResponseMessage{},
	}

	pb.RegisterServiceServer(grpcServer, Listener)

	// Make command line interface
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for {
			for scanner.Scan() {
				fmt.Print("Enter command: ")
				text := scanner.Text()
				// text = strings.Replace(text, "\n", "", -1)
				switch text {
				case "request":
					if state == "RELEASED" {
						state = "WANTED"
						log.Printf("Node %v is requesting critical section", *id)
						for i := 0; i < len(ports); i++ {
							if strings.Split(ports[i], ",")[0] != strconv.Itoa(*id) {
								conn, err := grpc.Dial("localhost:"+strings.Split(ports[i], ",")[1], grpc.WithBlock(), grpc.WithInsecure())
								if err != nil {
									log.Fatalf("Could not connect: %v", err)
								}

								c := pb.NewServiceClient(conn)
								response, err := c.Request(ctx, &pb.RequestMessage{Id: int32(*id), Timestamp: nodeTimestamp})
								if err != nil {
									log.Fatalf("Error when calling Request: %v", err)
								}
								log.Printf("Response from node %v: %v", strings.Split(ports[i], ",")[0], response.GetMessage())
								updateLamportTimestamp(response.GetTimestamp())
								if response.GetMessage() == "HELD" {
									log.Printf("Node %v is in critical section", *id)
									file, err := os.OpenFile(sharedFilePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
									if err != nil {
										log.Fatal(err)
									}
									file.WriteString("Node " + strconv.Itoa(*id) + " is in critical section\n")
									file.Close()
								}
								conn.Close()
							}

						}
					} else {
						log.Printf("Node %v is already requesting critical section", *id)
					}
				case "release":
					if state == "HELD" {
						state = "RELEASED"
						log.Printf("Node %v is releasing critical section", *id)
						for i := 0; i < len(queue); i++ {
							conn, err := grpc.Dial("localhost:"+strings.Split(ports[queue[i]], ",")[1], grpc.WithInsecure())
							if err != nil {
								log.Fatalf("Could not connect: %v", err)
							}
							c := pb.NewServiceClient(conn)
							response, err := c.Request(ctx, &pb.RequestMessage{Id: int32(*id), Timestamp: nodeTimestamp})
							if err != nil {
								log.Fatalf("Error when calling Request: %v", err)
							}
							log.Printf("Response from node %v: %v", strings.Split(ports[queue[i]], ",")[0], response.GetMessage())
							updateLamportTimestamp(response.GetTimestamp())
							if response.GetMessage() == "HELD" {
								log.Printf("Node %v is in critical section", *id)
								file, err := os.OpenFile(sharedFilePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
								if err != nil {
									log.Fatal(err)
								}
								file.WriteString("Node " + string(*id) + " is in critical section\n")
								file.Close()
							}
							conn.Close()
						}
					} else {
						log.Printf("Node %v is not in critical section", *id)
					}
				case "exit":
					log.Printf("Node %v is exiting", *id)
					os.Exit(0)
				default:
					log.Printf("Invalid command")

				}
			}
		}
	}()

	// Start listening for requests
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

}
