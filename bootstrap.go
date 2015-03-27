package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/fsouza/go-dockerclient"
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

func bootstrap(c *cli.Context) {

	if len(c.Args()) != 1 {
		log.Fatalf("[bootstrap] Nodes must be provided as a comma separated list or in a file. See '%s bootstrap --help'.", c.App.Name)
	}

	nodesAsString := c.Args()[0]
	consulServerCount := c.Int(flConsulServers.Name)
	nodes := strings.Split(nodesAsString, ",")

	log.Infof("[bootstrap] Started Swarm bootstrapping with parameters. Consul servers: %v, Nodes: %s", c.Int(flConsulServers.Name), nodesAsString)
	log.Debug("Creating docker client with docker.sock.")
	client, _ := docker.NewClient("unix:///var/run/docker.sock")

	tmpSwarmManagerID := startSwarmManagerContainer(client, TmpSwarmContainerName, nodesAsString, false)

	tmpSwarmManagerContainer, _ := client.InspectContainer(tmpSwarmManagerID)
	log.Debug("Wait 3 seconds for swarm manager.")
	time.Sleep(3000 * time.Millisecond)

	log.Debug("Creating docker client for temporary Swarm Manager.")
	tmpSwarmClient, _ := docker.NewClient("http://" + tmpSwarmManagerContainer.NetworkSettings.IPAddress + ":3376")

	var swarmNodes []*docker.SwarmNode

	firstNode := startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-0"), First, consulServerCount, "")
	swarmNodes = append(swarmNodes, firstNode)
	sem := make(chan empty, len(nodes)-1)
	for i := 1; i < len(nodes); i++ {
		go func(i int) {
			if i < consulServerCount {
				swarmNodes = append(swarmNodes, startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-%v", i), Server, consulServerCount, firstNode.IP))
			} else {
				swarmNodes = append(swarmNodes, startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-%v", i), Agent, consulServerCount, firstNode.IP))
			}
			sem <- empty{}
		}(i)
	}

	log.Debug("Wait for Consul containers to create on all nodes.")
	for i := 1; i < len(nodes); i++ {
		<-sem
	}

	sem = make(chan empty, len(nodes)-1)
	for i, node := range swarmNodes {
		go func(i int, node *docker.SwarmNode) {
			startSwarmAgentContainer(tmpSwarmClient, fmt.Sprintf("swarm-%v", i), node, firstNode.IP)
			sem <- empty{}
		}(i, node)
	}

	log.Debug("Wait for Swarm Agent containers to create on all nodes.")
	for i := 0; i < len(nodes); i++ {
		<-sem
	}

	time.Sleep(3000 * time.Millisecond)
	startSwarmManagerContainer(tmpSwarmClient, SwarmContainerName, "consul://"+firstNode.IP+":8500/swarm", true)

	log.Debug("Removing temporary Swarm manager.")
	client.RemoveContainer(docker.RemoveContainerOptions{ID: tmpSwarmManagerContainer.ID, Force: true})
	log.Info("Removed temporary Swarm manager.")

	log.Info("[bootstrap] Finished Swarm bootstrapping.")
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

	log.Debugf("Creating consul container with cmd: %s", createCmd)

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
	log.Debugf("Created consul container successfully, trying to start it.")
	client.StartContainer(container.ID, &hostConfig)
	container, _ = client.InspectContainer(container.ID)
	log.Infof("Started consul container on node: %s [Name: %s, ID: %s]", container.Node.IP, container.Name, container.ID)
	return container.Node
}

func startSwarmAgentContainer(client *docker.Client, name string, node *docker.SwarmNode, consulIP string) {
	log.Debugf("Creating swarm agent container on node %s with consul address: %s", node.IP, "consul://"+consulIP+":8500/swarm")
	config := docker.Config{
		Image: SwarmImage,
		Cmd:   []string{"join", "--addr=" + node.Addr, "consul://" + consulIP + ":8500/swarm"},
		Env:   []string{"constraint:node==" + node.Name},
	}
	container, _ := client.CreateContainer(docker.CreateContainerOptions{Name: name, Config: &config})
	log.Debug("Created swarm agent container successfully, trying to start it.")
	client.StartContainer(container.ID, &docker.HostConfig{})
	log.Info("Started swarm agent container on node: %s [Name: %s, ID: %s]", node.IP, container.Name, container.ID)
}

func startSwarmManagerContainer(client *docker.Client, name string, discoveryParam string, bindPort bool) string {
	log.Debugf("Creating swarm manager container with discovery parameter: %s", discoveryParam)
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
	log.Debug("Created swarm manager container successfully, trying to start it.")
	client.StartContainer(container.ID, &hostConfig)
	log.Infof("Started swarm manager container [Name: %s, ID: %s]", container.Name, container.ID)
	return container.ID
}
