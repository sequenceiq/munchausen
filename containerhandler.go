package main

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	docker "github.com/martonsereg/dockerclient"
	"strconv"
	"strings"
	"time"
)

func getSwarmNodes(client *docker.DockerClient) ([]*docker.SwarmNode, error) {
	if info, err := client.Info(); err != nil {
		return nil, fmt.Errorf("Failed to retrieve info from tmp swarm manager: %s", err)
	} else {
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
		return swarmNodes, nil
	}
}

func copyConsulConfigsDummy(i int) error {
	time.Sleep(1800 * time.Millisecond)
	if i == 1 || i == 2 {
		return fmt.Errorf("Error, error, error!!")
	}
	return nil
}

func runConsulConfigCopyContainer(client *docker.DockerClient, name string, node *docker.SwarmNode, consulServers []string) (string, error) {
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
	id, createErr := client.CreateContainer(config, name)
	if createErr != nil {
		log.Errorf("[bootstrap] Failed to create copy container: %s", createErr)
		return "", createErr
	}
	log.Debugf("[bootstrap] Created consul config copy container successfully, trying to start it. [ID: %s]", id)
	if startErr := client.StartContainer(id, &hostConfig); startErr != nil {
		log.Errorf("[bootstrap] Failed to start copy container: %s", startErr)
		return "", startErr
	}
	log.Infof("[bootstrap] Consul config copied to node: %s [ID: %s]", node.Name, id)
	return id, nil
}

func runConsulContainer(client *docker.DockerClient, name string) (string, error) {
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

	containerID, createErr := client.CreateContainer(config, name)
	if createErr != nil {
		log.Errorf("[bootstrap] Failed to create consul container: %s", createErr)
		return "", createErr
	}
	log.Debugf("[bootstrap] Created consul container successfully, trying to start it. [Name: %s]", name)
	if startErr := client.StartContainer(containerID, &hostConfig); startErr != nil {
		log.Errorf("[bootstrap] Failed to start copy container: %s", startErr)
		return "", startErr
	}
	log.Infof("[bootstrap] Started consul container [Name: %s, ID: %s]", name, containerID)
	return containerID, nil
}

func runSwarmAgentContainer(client *docker.DockerClient, name string, node *docker.SwarmNode, consulIP string) (string, error) {
	log.Debugf("[bootstrap] Creating swarm agent container on node %s with consul address: %s  [Name: %s]", node.Name, "consul://"+consulIP+":8500/swarm", name)
	hostConfig := docker.HostConfig{
		RestartPolicy: docker.RestartPolicy{Name: "always"},
	}
	config := &docker.ContainerConfig{
		Image:      SwarmImage,
		Cmd:        []string{"join", "--addr=" + node.Addr, "consul://" + consulIP + ":8500/swarm"},
		Env:        []string{"constraint:node==" + node.Name},
		HostConfig: hostConfig,
	}
	containerID, createErr := client.CreateContainer(config, name)
	if createErr != nil {
		log.Errorf("[bootstrap] Failed to create swarm agent container: %s", createErr)
		return "", createErr
	}
	log.Debugf("[bootstrap] Created swarm agent container successfully, trying to start it. [Name: %s]", name)
	if startErr := client.StartContainer(containerID, &hostConfig); startErr != nil {
		log.Errorf("[bootstrap] Failed to start swarm agent container: %s", startErr)
		return "", startErr
	}
	log.Infof("[bootstrap] Started swarm agent container on node: %s [Name: %s, ID: %s]", node.Name, name, containerID)
	return containerID, nil
}

func runSwarmManagerContainer(client *docker.DockerClient, name string, discoveryParam string, bindPort bool) (string, error) {
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
		ExposedPorts: exposedPorts,
		HostConfig:   hostConfig,
	}

	containerID, createErr := client.CreateContainer(config, name)
	if createErr != nil {
		log.Errorf("[bootstrap] Failed to create Swarm manager container: %s", createErr)
		return "", createErr
	}
	log.Debugf("[bootstrap] Created swarm manager container successfully, trying to start it.  [Name: %s]", name)
	if startErr := client.StartContainer(containerID, &hostConfig); startErr != nil {
		log.Errorf("[bootstrap] Failed to start Swarm manager container: %s", startErr)
		return "", startErr
	}
	log.Infof("[bootstrap] Started swarm manager container [Name: %s, ID: %s]", name, containerID)
	return containerID, nil
}
