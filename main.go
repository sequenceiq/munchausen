package main

import (
	"flag"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"log"
	"strconv"
	"strings"
	"time"
)

type empty struct{}
type ConsulNode int

const ConsulImage = "sequenceiq/consul:v0.4.1.ptr"

const (
	First ConsulNode = iota
	Server
	Agent
)

func main() {

	log.Println("[main] Launched swarm-bootstrap application")

	var consulServerCount int
	flag.IntVar(&consulServerCount, "consulServers", 3, "the number of Consul servers")
	flag.Parse()
	//if len(flag.Args()>0) -> error
	nodesAsString := flag.Args()[0]
	nodes := strings.Split(flag.Args()[0], ",")

	log.Println("Consul servers:", consulServerCount)
	log.Println("Nodes:", nodesAsString)

	endpoint := "unix:///var/run/docker.sock"
	client, _ := docker.NewClient(endpoint)

	config := docker.Config{
		Image: "swarm",
		Cmd:   []string{"manage", "-H", "tcp://0.0.0.0:3376", nodesAsString},
	}
	container, _ := client.CreateContainer(docker.CreateContainerOptions{Name: "tmp-swarm-mgr", Config: &config})
	client.StartContainer(container.ID, &docker.HostConfig{})

	container, _ = client.InspectContainer(container.ID)
	log.Println("Wait for swarm manager.")
	time.Sleep(3000 * time.Millisecond)
	log.Println("Started temporary Swarm manager.", container.ID, container.NetworkSettings.IPAddress)

	log.Println("http://" + container.NetworkSettings.IPAddress + ":3376")
	tmpSwarmClient, _ := docker.NewClient("http://" + container.NetworkSettings.IPAddress + ":3376")

	var swarmNodes []*docker.SwarmNode
	log.Println(swarmNodes)

	firstNode := startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-0"), First, consulServerCount, "")
	swarmNodes = append(swarmNodes, firstNode)
	log.Println(swarmNodes)
	for i := 1; i < len(nodes); i++ {
		if i < consulServerCount {
			swarmNodes = append(swarmNodes, startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-%v", i), Server, consulServerCount, firstNode.IP))
			log.Println(swarmNodes)
		} else {
			swarmNodes = append(swarmNodes, startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-%v", i), Agent, consulServerCount, firstNode.IP))
			log.Println(swarmNodes)
		}
	}

	log.Println(swarmNodes)

	log.Println("Finished Swarm bootstrapping")

}

func startConsulContainer(client *docker.Client, containerName string, nodeType ConsulNode, serverCount int, joinIp string) *docker.SwarmNode {
	var createCmd []string
	switch nodeType {
	case First:
		createCmd = []string{"-server", "-bootstrap-expect", strconv.Itoa(serverCount)}
	case Server:
		createCmd = []string{"-server", "-bootstrap-expect", strconv.Itoa(serverCount), "-retry-join", joinIp}
	case Agent:
		createCmd = []string{"-retry-join", joinIp}
	}
	consulConfig := docker.Config{
		Image: ConsulImage,
		Cmd:   createCmd,
	}
	portBindings := make(map[docker.Port][]docker.PortBinding)
	portBindings["tcp/8500"] = []docker.PortBinding{docker.PortBinding{HostIP: "0.0.0.0", HostPort: "8500"}}
	portBindings["tcp/8400"] = []docker.PortBinding{docker.PortBinding{HostIP: "0.0.0.0", HostPort: "8400"}}

	consulHostConfig := docker.HostConfig{
		NetworkMode:   "host",
		RestartPolicy: docker.RestartPolicy{Name: "always"},
		PortBindings:  portBindings,
	}
	container, _ := client.CreateContainer(docker.CreateContainerOptions{Name: containerName, Config: &consulConfig, HostConfig: &consulHostConfig})
	client.StartContainer(container.ID, &consulHostConfig)
	container, _ = client.InspectContainer(container.ID)
	log.Println("Started consul container", container.Name, container.ID)
	return container.Node
}
