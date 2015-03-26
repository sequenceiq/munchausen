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
const SwarmImage = "swarm"

const TmpSwarmContainerName = "tmp-swarm-manager"
const SwarmContainerName = "swarm-manager"

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

	client, _ := docker.NewClient("unix:///var/run/docker.sock")

	tmpSwarmManagerID := startSwarmManagerContainer(client, TmpSwarmContainerName, nodesAsString, false)

	tmpSwarmManagerContainer, _ := client.InspectContainer(tmpSwarmManagerID)
	log.Println("Wait for swarm manager.")
	time.Sleep(3000 * time.Millisecond)

	tmpSwarmClient, _ := docker.NewClient("http://" + tmpSwarmManagerContainer.NetworkSettings.IPAddress + ":3376")

	var swarmNodes []*docker.SwarmNode

	firstNode := startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-0"), First, consulServerCount, "")
	swarmNodes = append(swarmNodes, firstNode)
	for i := 1; i < len(nodes); i++ {
		if i < consulServerCount {
			swarmNodes = append(swarmNodes, startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-%v", i), Server, consulServerCount, firstNode.IP))
		} else {
			swarmNodes = append(swarmNodes, startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-%v", i), Agent, consulServerCount, firstNode.IP))
		}
	}

	for i, node := range swarmNodes {
		startSwarmAgentContainer(tmpSwarmClient, fmt.Sprintf("swarm-%v", i), node, firstNode.IP)
	}

	time.Sleep(3000 * time.Millisecond)
	startSwarmManagerContainer(tmpSwarmClient, SwarmContainerName, "consul://"+firstNode.IP+":8500/swarm", true)

	log.Println("Wait for new swarm manager.")
	time.Sleep(3000 * time.Millisecond)

	log.Println("Removing temporary Swarm manager.")
	swarmClient, _ := docker.NewClient("http://127.0.0.1:3376")
	swarmClient.RemoveContainer(docker.RemoveContainerOptions{ID: tmpSwarmManagerContainer.ID})

	log.Println("Finished Swarm bootstrapping")

}

func startConsulContainer(client *docker.Client, name string, nodeType ConsulNode, serverCount int, joinIp string) *docker.SwarmNode {
	var createCmd []string
	switch nodeType {
	case First:
		createCmd = []string{"-server", "-bootstrap-expect", strconv.Itoa(serverCount)}
	case Server:
		createCmd = []string{"-server", "-bootstrap-expect", strconv.Itoa(serverCount), "-retry-join", joinIp}
	case Agent:
		createCmd = []string{"-retry-join", joinIp}
	}

	exposedPorts := make(map[docker.Port]struct{})
	var empty struct{}
	exposedPorts["8500/tcp"] = empty
	exposedPorts["8400/tcp"] = empty

	config := docker.Config{
		Image:        ConsulImage,
		Cmd:          createCmd,
		ExposedPorts: exposedPorts,
	}
	portBindings := make(map[docker.Port][]docker.PortBinding)
	portBindings["8500/tcp"] = []docker.PortBinding{docker.PortBinding{HostIP: "0.0.0.0", HostPort: "8500"}}
	portBindings["8400/tcp"] = []docker.PortBinding{docker.PortBinding{HostIP: "0.0.0.0", HostPort: "8400"}}

	hostConfig := docker.HostConfig{
		NetworkMode:   "host",
		RestartPolicy: docker.RestartPolicy{Name: "always"},
		PortBindings:  portBindings,
	}
	container, _ := client.CreateContainer(docker.CreateContainerOptions{Name: name, Config: &config, HostConfig: &hostConfig})
	client.StartContainer(container.ID, &hostConfig)
	container, _ = client.InspectContainer(container.ID)
	log.Println("Started consul container", container.Name, container.ID)
	return container.Node
}

func startSwarmAgentContainer(client *docker.Client, name string, node *docker.SwarmNode, consulIP string) {
	config := docker.Config{
		Image: SwarmImage,
		Cmd:   []string{"join", "--addr=" + node.Addr, "consul://" + consulIP + ":8500/swarm"},
		Env:   []string{"constraint:node==" + node.Name},
	}
	container, _ := client.CreateContainer(docker.CreateContainerOptions{Name: name, Config: &config})
	client.StartContainer(container.ID, &docker.HostConfig{})
	log.Println("Started swarm agent container", container.Name, container.ID)
}

func startSwarmManagerContainer(client *docker.Client, name string, discoveryParam string, bindPort bool) string {
	hostConfig := docker.HostConfig{}
	if bindPort {
		portBindings := make(map[docker.Port][]docker.PortBinding)
		portBindings["3376/tcp"] = []docker.PortBinding{docker.PortBinding{HostIP: "0.0.0.0", HostPort: "3376"}}
		hostConfig = docker.HostConfig{
			PortBindings: portBindings,
		}
	}

	exposedPorts := make(map[docker.Port]struct{})
	var empty struct{}
	exposedPorts["3376/tcp"] = empty

	config := docker.Config{
		Image:        SwarmImage,
		Cmd:          []string{"--debug", "manage", "-H", "tcp://0.0.0.0:3376", discoveryParam},
		Env:          []string{"affinity:container==" + TmpSwarmContainerName},
		ExposedPorts: exposedPorts,
	}

	container, _ := client.CreateContainer(docker.CreateContainerOptions{Name: name, Config: &config, HostConfig: &hostConfig})
	client.StartContainer(container.ID, &hostConfig)
	log.Println("Started swarm manager container", container.Name, container.ID)
	return container.ID
}
