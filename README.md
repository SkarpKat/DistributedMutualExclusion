# DistributedMutualExclusion

## Logs
Every node will make it's own logfile. The program is made with a multiwriter so it will write to it's own logfile and to the terminal.

## How to use
To use the program you need to run 3 instances of the client script. What's important is that you stand the the DistributedMutualExclusion folder. When standing in the project root folder run the command ```go run ./Client/Client.go -id```. There is a flag id where you put the id of the client. It's important to note that it should match the ports.txt file with id and port.
We recommend to run 
<br>
```go run ./Client/Client.go -id 0``` 
<br>```go run ./Client/Client.go -id 1``` 
<br>```go run ./Client/Client.go -id 2```<br>
When the clients is running you can type ```request```. If you can't type that means that you are enqueued and waiting. The client that can type can type to the shared file called sharedFile.log and if you type exit you will release and the next in line can enter.
