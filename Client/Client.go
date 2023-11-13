// client.go
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	pb "github.com/SkarpKat/DistributedMutualExclusion/proto"
	"google.golang.org/grpc"
)

type client struct {
	pb.UnimplementedServiceServer
	id                string
	addresses         []string
	mu                sync.Mutex
	inCriticalSection bool
	waitingQueue      []string
	lamportTimestamp  int64
}

func (c *client) RequestCriticalSection(ctx context.Context, req *pb.RequestCriticalSectionRequest) (*pb.RequestCriticalSectionResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.inCriticalSection && len(c.waitingQueue) == 0 {
		c.inCriticalSection = true
		c.lamportTimestamp++
		return &pb.RequestCriticalSectionResponse{Granted: true}, nil
	}

	// If not in the critical section or there are clients in the waiting queue, add the client to the waiting queue
	c.waitingQueue = append(c.waitingQueue, req.ClientId)
	return &pb.RequestCriticalSectionResponse{Granted: false}, nil
}

func (c *client) ReleaseCriticalSection(ctx context.Context, req *pb.ReleaseCriticalSectionRequest) (*pb.ReleaseCriticalSectionResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Release the critical section and notify the next client in the waiting queue
	c.inCriticalSection = false

	if len(c.waitingQueue) > 0 {
		nextClient := c.waitingQueue[0]
		c.waitingQueue = c.waitingQueue[1:]

		// Notify the next client in the waiting queue
		conn, err := grpc.Dial(fmt.Sprintf(":%s", nextClient), grpc.WithInsecure())
		if err != nil {
			log.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()

		client := pb.NewServiceClient(conn)
		c.lamportTimestamp++
		_, err = client.RequestCriticalSection(context.Background(), &pb.RequestCriticalSectionRequest{ClientId: nextClient, LamportTimestamp: c.lamportTimestamp})
		if err != nil {
			log.Fatalf("Failed to notify next client: %v", err)
		}
	}
	return &pb.ReleaseCriticalSectionResponse{}, nil
}

func (c *client) enterCriticalSection() {
	// For simplicity, pick a random address from the list
	peerAddress := c.addresses[0]

	conn, err := grpc.Dial(peerAddress, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewServiceClient(conn)

	req := &pb.RequestCriticalSectionRequest{ClientId: c.id, LamportTimestamp: c.lamportTimestamp}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	res, err := client.RequestCriticalSection(ctx, req)
	if err != nil {
		log.Fatalf("Failed to request critical section: %v", err)
	}

	if res.Granted {
		fmt.Printf("Client %s entered the critical section with Lamport timestamp %d\n", c.id, c.lamportTimestamp)
		time.Sleep(2 * time.Second)                                                                                            // Simulate work in the critical section
		c.ReleaseCriticalSection(ctx, &pb.ReleaseCriticalSectionRequest{ClientId: c.id, LamportTimestamp: c.lamportTimestamp}) // Release the critical section
	} else {
		fmt.Printf("Client %s is waiting for the critical section\n", c.id)
	}
}

func main() {
	clientID := os.Args[1] // Get client ID from command line argument

	// For simplicity, a list of addresses is predefined
	addresses := []string{"localhost:50051", "localhost:50052", "localhost:50053"}

	client := &client{id: clientID, addresses: addresses}

	listen, err := net.Listen("tcp", fmt.Sprintf(":%s", clientID))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterServiceServer(grpcServer, client)

	fmt.Printf("Client %s is running on port :%s\n", clientID, clientID)

	go func() {
		if err := grpcServer.Serve(listen); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Command-line interface for runtime commands
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		command := scanner.Text()

		switch command {
		case "request":
			client.enterCriticalSection()
		case "release":
			client.ReleaseCriticalSection(context.Background(), &pb.ReleaseCriticalSectionRequest{})
		default:
			fmt.Println("Invalid command. Available commands: 'request', 'release'")
		}
	}
}
