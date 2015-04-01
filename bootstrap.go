package main

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	docker "github.com/martonsereg/dockerclient"
	"strconv"
	"strings"
	"time"
)

func bootstrap(c *cli.Context) {

	if len(c.Args()) != 1 {
		log.Fatalf("[bootstrap] Nodes must be provided as a comma separated list. See '%s bootstrap --help'.", c.App.Name)
	}

	nodesAsString := c.Args()[0]
	consulServers := strings.Split(c.String(flConsulServers.Name), ",")

	log.Infof("[bootstrap] Started Swarm bootstrapping with parameters. Consul servers: %v, Nodes: %s", len(consulServers), nodesAsString)
	log.Debug("[bootstrap] Creating docker client with docker.sock.")
	client, _ := docker.NewDockerClient("unix:///var/run/docker.sock", nil)

	tmpSwarmManagerID := startSwarmManagerContainer(client, TmpSwarmContainerName, nodesAsString, false)
	tmpSwarmManagerContainer, _ := client.InspectContainer(tmpSwarmManagerID)
	log.Debug("[bootstrap] Wait 3 seconds for swarm manager.")
	time.Sleep(3000 * time.Millisecond)
	log.Debug("[bootstrap] Creating docker client for temporary Swarm Manager.")
	tmpSwarmClient, _ := docker.NewDockerClient("http://"+tmpSwarmManagerContainer.NetworkSettings.IpAddress+":3376", nil)

	swarmNodes := getSwarmNodes(tmpSwarmClient)

	sem := make(chan empty, len(swarmNodes))
	for _, node := range swarmNodes {
		go func(node *docker.SwarmNode) {
			copyConsulConfigs(tmpSwarmClient, node, consulServers)
			sem <- empty{}
		}(node)
	}

	log.Debug("[bootstrap] Wait for consul configurations to be ready on all nodes.")
	for i := 0; i < len(swarmNodes); i++ {
		<-sem
	}

	sem = make(chan empty, len(swarmNodes))
	for i, _ := range swarmNodes {
		go func(i int) {
			startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-%v", i))
			sem <- empty{}
		}(i)
	}

	log.Debug("[bootstrap] Wait for Consul containers to create on all nodes.")
	for i := 0; i < len(swarmNodes); i++ {
		<-sem
	}

	sem = make(chan empty, len(swarmNodes))
	for i, node := range swarmNodes {
		go func(i int, node *docker.SwarmNode) {
			startSwarmAgentContainer(tmpSwarmClient, fmt.Sprintf("swarm-%v", i), node, consulServers[0])
			sem <- empty{}
		}(i, node)
	}

	log.Debug("[bootstrap] Wait for Swarm Agent containers to create on all nodes.")
	for i := 0; i < len(swarmNodes); i++ {
		<-sem
	}

	time.Sleep(3000 * time.Millisecond)
	startSwarmManagerContainer(tmpSwarmClient, SwarmContainerName, "consul://"+consulServers[0]+":8500/swarm", true)

	log.Debug("[bootstrap] Removing temporary Swarm manager.")
	client.RemoveContainer(tmpSwarmManagerContainer.Id, true, true)
	log.Info("[bootstrap] Removed temporary Swarm manager.")
	log.Info("[bootstrap] Finished Swarm bootstrapping.")
	log.Info("[bootstrap] To try the new Swarm Manager run 'docker -H tcp://127.0.0.1:3376 info'")
}

func getSwarmNodes(client *docker.DockerClient) []*docker.SwarmNode {
	info, _ := client.Info()
	var swarmNodes []*docker.SwarmNode
	// Swarm returns nodes and their info in a 2 dimensional json array basically unstructured
	// The first array contains the text "Nodes" and the number of the nodes, then comes the nodes in the following 4 element blocks:
	// [name, addr],[" └ Containers", containers],[" └ Reserved CPUs", cpu],[" └ Reserved Memory", memory],[name, addr],[" └ Containers"],...
	if info.DriverStatus[0][0] == "\bNodes" {
		nodeCount, _ := strconv.Atoi(info.DriverStatus[0][1])
		for i := 0; i < nodeCount; i++ {
			swarmNodes = append(swarmNodes, &docker.SwarmNode{
				Addr: info.DriverStatus[i*4+1][1],
				Name: info.DriverStatus[i*4+1][0],
			})
		}
	}
	log.Infof("[bootstrap] Temporary Swarm manager found %v nodes", len(swarmNodes))
	return swarmNodes
}

func copyConsulConfigs(client *docker.DockerClient, node *docker.SwarmNode, consulServers []string) {
	log.Debugf("[bootstrap] Creating consul configuration file for node %s.", node.Name)
	server := false
	var joinIPs []string
	for _, consulServer := range consulServers {
		if strings.Contains(node.Addr, consulServer) {
			server = true
		} else {
			joinIPs = append(joinIPs, consulServer)
		}
	}
	log.Debugf("[bootstrap] RetryJoin IPs for node %s: %s", node.Name, joinIPs)
	consulConfig := ConsulConfig{
		AdvertiseAddr:      strings.Split(node.Addr, ":")[0],
		DataDir:            "/data",
		UiDir:              "/ui",
		ClientAddr:         "0.0.0.0",
		DNSRecursor:        "8.8.8.8",
		DisableUpdateCheck: true,
		RetryJoin:          joinIPs,
		Ports: PortConfig{
			DNS:   53,
			HTTP:  8500,
			HTTPS: -1,
		},
	}
	if server {
		log.Debugf("[bootstrap] Node %s is a server, adding bootstrap_expect: %v and server: true configuration options.", node.Name, len(consulServers))
		consulConfig.BootstrapExpect = len(consulServers)
		consulConfig.Server = true
	}
	hostConfig := docker.HostConfig{
		Binds: []string{"/etc/consul:/config"},
	}
	consulConfigJson, _ := json.MarshalIndent(consulConfig, "", "  ")
	log.Debugf("[bootstrap] Consul configuration file created for node %s", node.Name)
	config := &docker.ContainerConfig{
		Image:      "gliderlabs/alpine:3.1",
		Cmd:        []string{"sh", "-c", "echo '" + string(consulConfigJson) + "' > /config/consul.json && cat /config/consul.json"},
		Env:        []string{"constraint:node==" + node.Name},
		HostConfig: hostConfig,
	}
	id, _ := client.CreateContainer(config, "")
	client.StartContainer(id, &hostConfig)
	log.Infof("[bootstrap] Consul config copied to node: %s [ID: %s]", node.Name, id)
}

func startConsulContainer(client *docker.DockerClient, name string) *docker.SwarmNode {
	log.Debugf("[bootstrap] Creating consul container [Name: %s]", name)

	portBindings := make(map[string][]docker.PortBinding)
	portBindings["8500/tcp"] = []docker.PortBinding{docker.PortBinding{HostIp: "0.0.0.0", HostPort: "8500"}}
	portBindings["8400/tcp"] = []docker.PortBinding{docker.PortBinding{HostIp: "0.0.0.0", HostPort: "8400"}}

	hostConfig := docker.HostConfig{
		Binds:         []string{"/etc/consul/consul.json:/config/consul.json"},
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
		ExposedPorts: exposedPorts,
		HostConfig:   hostConfig,
	}

	containerID, _ := client.CreateContainer(config, name)
	log.Debugf("[bootstrap] Created consul container successfully, trying to start it. [Name: %s]", name)
	client.StartContainer(containerID, &hostConfig)
	container, _ := client.InspectContainer(containerID)
	log.Infof("[bootstrap] Started consul container on node: %s [Name: %s, ID: %s]", container.Node.Name, container.Name, container.Id)
	return container.Node
}

func startSwarmAgentContainer(client *docker.DockerClient, name string, node *docker.SwarmNode, consulIP string) {
	log.Debugf("[bootstrap] Creating swarm agent container on node %s with consul address: %s  [Name: %s]", node.Name, "consul://"+consulIP+":8500/swarm", name)
	config := &docker.ContainerConfig{
		Image: SwarmImage,
		Cmd:   []string{"join", "--addr=" + node.Addr, "consul://" + consulIP + ":8500/swarm"},
		Env:   []string{"constraint:node==" + node.Name},
	}
	containerID, _ := client.CreateContainer(config, name)
	log.Debugf("[bootstrap] Created swarm agent container successfully, trying to start it. [Name: %s]", name)
	client.StartContainer(containerID, &docker.HostConfig{})
	log.Infof("[bootstrap] Started swarm agent container on node: %s [Name: %s, ID: %s]", node.Name, name, containerID)
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
