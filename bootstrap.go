package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	docker "github.com/martonsereg/dockerclient"
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
		log.Fatalf("[bootstrap] Nodes must be provided as a comma separated list. See '%s bootstrap --help'.", c.App.Name)
	}

	nodesAsString := c.Args()[0]
	consulServerCount := c.Int(flConsulServers.Name)
	nodes := strings.Split(nodesAsString, ",")

	log.Infof("[bootstrap] Started Swarm bootstrapping with parameters. Consul servers: %v, Nodes: %s", c.Int(flConsulServers.Name), nodesAsString)
	log.Debug("[bootstrap] Creating docker client with docker.sock.")
	client, _ := docker.NewDockerClient("unix:///var/run/docker.sock", nil)

	tmpSwarmManagerID := startSwarmManagerContainer(client, TmpSwarmContainerName, nodesAsString, false)

	tmpSwarmManagerContainer, _ := client.InspectContainer(tmpSwarmManagerID)
	log.Debug("[bootstrap] Wait 3 seconds for swarm manager.")
	time.Sleep(3000 * time.Millisecond)

	log.Debug("[bootstrap] Creating docker client for temporary Swarm Manager.")
	tmpSwarmClient, _ := docker.NewDockerClient("http://"+tmpSwarmManagerContainer.NetworkSettings.IpAddress+":3376", nil)

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

	log.Debug("[bootstrap] Wait for Consul containers to create on all nodes.")
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

	log.Debug("[bootstrap] Wait for Swarm Agent containers to create on all nodes.")
	for i := 0; i < len(nodes); i++ {
		<-sem
	}

	time.Sleep(3000 * time.Millisecond)
	startSwarmManagerContainer(tmpSwarmClient, SwarmContainerName, "consul://"+firstNode.IP+":8500/swarm", true)

	log.Debug("[bootstrap] Removing temporary Swarm manager.")
	client.RemoveContainer(tmpSwarmManagerContainer.Id, true, true)
	log.Info("[bootstrap] Removed temporary Swarm manager.")

	log.Info("[bootstrap] Finished Swarm bootstrapping.")
}

func startConsulContainer(client *docker.DockerClient, name string, nodeType ConsulNode, serverCount int, joinIp string) *docker.SwarmNode {
	var createCmd []string
	switch nodeType {
	case First:
		createCmd = []string{"-server", "-bootstrap-expect", strconv.Itoa(serverCount)}
	case Server:
		createCmd = []string{"-server", "-bootstrap-expect", strconv.Itoa(serverCount), "-retry-join", joinIp}
	case Agent:
		createCmd = []string{"-retry-join", joinIp}
	}

	log.Debugf("[bootstrap] Creating consul container with cmd: %s  [Name: %s]", createCmd, name)

	portBindings := make(map[string][]docker.PortBinding)
	portBindings["8500/tcp"] = []docker.PortBinding{docker.PortBinding{HostIp: "0.0.0.0", HostPort: "8500"}}
	portBindings["8400/tcp"] = []docker.PortBinding{docker.PortBinding{HostIp: "0.0.0.0", HostPort: "8400"}}

	hostConfig := docker.HostConfig{
		NetworkMode:   "host",
		RestartPolicy: docker.RestartPolicy{Name: "always"},
		PortBindings:  portBindings,
	}

	exposedPorts := make(map[string]struct{})
	var empty struct{}
	exposedPorts["8500/tcp"] = empty
	exposedPorts["8400/tcp"] = empty

	config := &docker.ContainerConfig{
		Image:        ConsulImage,
		Cmd:          createCmd,
		ExposedPorts: exposedPorts,
		HostConfig:   hostConfig,
	}

	containerID, _ := client.CreateContainer(config, name)
	log.Debugf("[bootstrap] Created consul container successfully, trying to start it. [Name: %s]", name)
	client.StartContainer(containerID, &hostConfig)
	container, _ := client.InspectContainer(containerID)
	log.Infof("[bootstrap] Started consul container on node: %s [Name: %s, ID: %s]", container.Node.IP, container.Name, container.Id)
	return container.Node
}

func startSwarmAgentContainer(client *docker.DockerClient, name string, node *docker.SwarmNode, consulIP string) {
	log.Debugf("[bootstrap] Creating swarm agent container on node %s with consul address: %s  [Name: %s]", node.IP, "consul://"+consulIP+":8500/swarm", name)
	config := &docker.ContainerConfig{
		Image: SwarmImage,
		Cmd:   []string{"join", "--addr=" + node.Addr, "consul://" + consulIP + ":8500/swarm"},
		Env:   []string{"constraint:node==" + node.Name},
	}
	containerID, _ := client.CreateContainer(config, name)
	log.Debugf("[bootstrap] Created swarm agent container successfully, trying to start it. [Name: %s]", name)
	client.StartContainer(containerID, &docker.HostConfig{})
	log.Infof("[bootstrap] Started swarm agent container on node: %s [Name: %s, ID: %s]", node.IP, name, containerID)
}

func startSwarmManagerContainer(client *docker.DockerClient, name string, discoveryParam string, bindPort bool) string {
	log.Debugf("[bootstrap] Creating swarm manager container with discovery parameter: %s", discoveryParam)
	hostConfig := docker.HostConfig{}
	if bindPort {
		portBindings := make(map[string][]docker.PortBinding)
		portBindings["3376/tcp"] = []docker.PortBinding{docker.PortBinding{HostIp: "0.0.0.0", HostPort: "3376"}}
		hostConfig = docker.HostConfig{
			PortBindings: portBindings,
		}
	}

	exposedPorts := make(map[string]struct{})
	var empty struct{}
	exposedPorts["3376/tcp"] = empty

	config := &docker.ContainerConfig{
		Image:        SwarmImage,
		Cmd:          []string{"--debug", "manage", "-H", "tcp://0.0.0.0:3376", discoveryParam},
		Env:          []string{"affinity:container==" + TmpSwarmContainerName},
		ExposedPorts: exposedPorts,
		HostConfig:   hostConfig,
	}

	containerID, _ := client.CreateContainer(config, name)
	log.Debugf("[bootstrap] Created swarm manager container successfully, trying to start it.  [Name: %s]", name)
	client.StartContainer(containerID, &hostConfig)
	log.Infof("[bootstrap] Started swarm manager container [Name: %s, ID: %s]", name, containerID)
	return containerID
}
