package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"io/ioutil"
	"regexp"
	log "github.com/Sirupsen/logrus"
	docker "github.com/samalba/dockerclient"
)

func getSwarmNodes(client *docker.DockerClient) ([]*SwarmNode, error) {
	if info, err := client.Info(); err != nil {
		return nil, fmt.Errorf("Failed to retrieve info from tmp swarm manager: %s", err)
	} else {
		var swarmNodes []*SwarmNode
		var nodeCount int
		// Swarm returns nodes and their info in a 2 dimensional json array basically unstructured
		// The first three arrays contain the Strategy ("\bStrategy"), Filters ("\bFilters") and Nodes ("\bNodes") respectively, then comes the nodes in the following 4 element blocks:
		// [name, addr],[" └ Containers", containers],[" └ Reserved CPUs", cpu],[" └ Reserved Memory", memory],[name, addr],[" └ Containers"],...
		for i := 0; i < len(info.DriverStatus); i++ {
			if info.DriverStatus[i][0] == "\u0008Nodes" {
				nodeCount, _ = strconv.Atoi(info.DriverStatus[i][1])
				break
			}
		}
		for i := 0; i < nodeCount; i++ {
			node := info.DriverStatus[i*5+4]
			name := node[0]
			address := node[1]
			swarmNodes = append(swarmNodes, &SwarmNode{
				IP: strings.Split(address, ":")[0],
				Addr: address,
				Name: name,
			})
		}
		log.Infof("[containerhandler] Temporary Swarm manager found %v nodes", len(swarmNodes))
		return swarmNodes, nil
	}
}

func determineDNSRecursors(fallbackDNSRecursors []string) ([]string) {
	var dnsRecursors []string
	if dat, err := ioutil.ReadFile("/etc/resolv.conf"); err == nil {
		resolvContent := string(dat)
		log.Debugf("[containerhandler] Loaded /etc/resolv.conf file: %s.", resolvContent)
		r, _ := regexp.Compile("nameserver .*")
		if nameserverLines := r.FindAllString(resolvContent, -1); nameserverLines != nil {
			for _, nameserverLine := range nameserverLines {
				log.Debugf("[containerhandler] Found nameserverline: %s.", nameserverLine)
				dnsRecursor := strings.TrimSpace(strings.Split(nameserverLine, " ")[1])
				log.Debugf("[containerhandler] Parsed DNS recursor: %s.", dnsRecursor)
				dnsRecursors = append(dnsRecursors, dnsRecursor)
			}
		}
	} else {
		log.Warnf("[containerhandler] Failed to load /etc/resolv.conf")
	}
	if (fallbackDNSRecursors != nil) {
		dnsRecursors = append(dnsRecursors, fallbackDNSRecursors...)
	}
	return dnsRecursors
}

func runConsulConfigCopyContainer(client *docker.DockerClient, name string, node *SwarmNode, consulServers []string, consulLogLocation string, fallbackDNSRecursors []string) (string, error) {
	name = fmt.Sprintf("%s-%s", node.Name, name)
	log.Debugf("[containerhandler] Creating consul configuration file for node %s.", node.Name)
	server := false
	var joinIPs []string
	for _, consulServer := range consulServers {
		if strings.Contains(node.Addr, consulServer) {
			server = true
		} else {
			joinIPs = append(joinIPs, consulServer)
		}
	}

	dnsRecursors := determineDNSRecursors(fallbackDNSRecursors)
	log.Infof("[containerhandler] Used DNSRecursor : %s.", dnsRecursors)

	log.Debugf("[containerhandler] RetryJoin IPs for node %s: %s", node.Name, joinIPs)
	consulConfig := ConsulConfig{
		AdvertiseAddr:      strings.Split(node.Addr, ":")[0],
		DataDir:            "/data",
		UiDir:              "/ui",
		ClientAddr:         "0.0.0.0",
		DNSRecursors:        dnsRecursors,
		DisableUpdateCheck: true,
		RetryJoin:          joinIPs,
		Ports: PortConfig{
			DNS:   53,
			HTTP:  8500,
			HTTPS: -1,
		},
		DNS: DNSConfig{
			AllowStale: true,
			MaxStale:   "5m",
			NodeTTL:    "1m",
		},
	}
	if server {
		log.Debugf("[containerhandler] Node %s is a server, adding bootstrap_expect: %v and server: true configuration options.", node.Name, len(consulServers))
		consulConfig.BootstrapExpect = len(consulServers)
		consulConfig.Server = true
	}
	bindsArray := []string{"/etc/consul:/config"}
	if len(consulLogLocation) != 0 {
		bindsArray = append(bindsArray, fmt.Sprintf("%s:/var/log/consul", consulLogLocation))
	}
	hostConfig := docker.HostConfig{
		Binds: bindsArray,
	}
	consulConfigJson, _ := json.MarshalIndent(consulConfig, "", "  ")
	log.Debugf("[containerhandler] Consul configuration file created for node %s", node.Name)
	config := &docker.ContainerConfig{
		Image:      "gliderlabs/alpine:3.1",
		Cmd:        []string{"sh", "-c", "echo '" + string(consulConfigJson) + "' > /config/consul.json && cat /config/consul.json"},
		Env:        []string{"constraint:node==" + node.Name},
		HostConfig: hostConfig,
	}
	if err := client.RemoveContainer(fmt.Sprintf("%s/%s", node.Name, name), true, true); err != nil {
		log.Debugf("Couldn't remove container: %s/%s: %s", node.Name, name, err)
	} else {
		log.Debugf("Force removed container with name %s/%s.", node.Name, name)
	}
	id, createErr := client.CreateContainer(config, name)
	if createErr != nil {
		log.Errorf("[containerhandler] Failed to create copy container: %s", createErr)
		return "", createErr
	}
	log.Debugf("[containerhandler] Created consul config copy container successfully, trying to start it. [ID: %s]", id)
	if startErr := client.StartContainer(id, &hostConfig); startErr != nil {
		log.Errorf("[containerhandler] Failed to start copy container: %s", startErr)
		return "", startErr
	}
	log.Infof("[containerhandler] Consul config copied to node: %s [ID: %s]", node.Name, id)
	return id, nil
}

func runConsulContainer(client *docker.DockerClient, name string, node *SwarmNode, consulLogLocation string) (string, error) {
	name = fmt.Sprintf("%s-%s", node.Name, name)
	log.Debugf("[containerhandler] Creating consul container [Name: %s]", name)

	portBindings := make(map[string][]docker.PortBinding)
	portBindings["8500/tcp"] = []docker.PortBinding{docker.PortBinding{HostIp: "0.0.0.0", HostPort: "8500"}}
	portBindings["8400/tcp"] = []docker.PortBinding{docker.PortBinding{HostIp: "0.0.0.0", HostPort: "8400"}}

	bindsArray := []string{"/etc/consul/consul.json:/config/consul.json"}
	if len(consulLogLocation) != 0 {
		bindsArray = append(bindsArray, fmt.Sprintf("%s:/var/log/consul", consulLogLocation))
	}
	hostConfig := docker.HostConfig{
		Binds:         bindsArray,
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
		Env:          []string{"constraint:node==" + node.Name},
		HostConfig:   hostConfig,
	}
	if err := client.RemoveContainer(fmt.Sprintf("%s/%s", node.Name, name), true, true); err != nil {
		log.Debugf("Couldn't remove container: %s/%s: %s", node.Name, name, err)
	} else {
		log.Debugf("Force removed container with name %s/%s.", node.Name, name)
	}
	containerID, createErr := client.CreateContainer(config, name)
	if createErr != nil {
		log.Errorf("[containerhandler] Failed to create consul container: %s", createErr)
		return "", createErr
	}
	log.Debugf("[containerhandler] Created consul container successfully, trying to start it. [Name: %s]", name)
	if startErr := client.StartContainer(containerID, &hostConfig); startErr != nil {
		log.Errorf("[containerhandler] Failed to start copy container: %s", startErr)
		return "", startErr
	}
	log.Infof("[containerhandler] Started consul container [Name: %s, ID: %s]", name, containerID)
	return containerID, nil
}

func runSwarmAgentContainer(client *docker.DockerClient, name string, node *SwarmNode, consulIP string) (string, error) {
	name = fmt.Sprintf("%s-%s", node.Name, name)
	log.Debugf("[containerhandler] Creating swarm agent container on node %s with consul address: %s  [Name: %s]", node.Name, "consul://" + node.IP + ":8500/swarm", name)
	hostConfig := docker.HostConfig{
		RestartPolicy: docker.RestartPolicy{Name: "always"},
	}
	config := &docker.ContainerConfig{
		Image:      SwarmImage,
		Cmd:        []string{"join", "--addr=" + node.Addr, "consul://" + node.IP + ":8500/swarm"},
		Env:        []string{"constraint:node==" + node.Name},
		HostConfig: hostConfig,
	}
	if err := client.RemoveContainer(fmt.Sprintf("%s/%s", node.Name, name), true, true); err != nil {
		log.Debugf("Couldn't remove container: %s/%s: %s", node.Name, name, err)
	} else {
		log.Debugf("Force removed container with name %s/%s.", node.Name, name)
	}
	containerID, createErr := client.CreateContainer(config, name)
	if createErr != nil {
		log.Errorf("[containerhandler] Failed to create swarm agent container: %s", createErr)
		return "", createErr
	}
	log.Debugf("[containerhandler] Created swarm agent container successfully, trying to start it. [Name: %s]", name)
	if startErr := client.StartContainer(containerID, &hostConfig); startErr != nil {
		log.Errorf("[containerhandler] Failed to start swarm agent container: %s", startErr)
		return "", startErr
	}
	log.Infof("[containerhandler] Started swarm agent container on node: %s [Name: %s, ID: %s]", node.Name, name, containerID)
	return containerID, nil
}

func runSwarmManagerContainer(client *docker.DockerClient, name string, discoveryParam string, bindPort bool) (string, error) {
	log.Debugf("[containerhandler] Creating swarm manager container with discovery parameter: %s", discoveryParam)
	hostConfig := docker.HostConfig{
		RestartPolicy: docker.RestartPolicy{Name: "always"},
	}
	if bindPort {
		portBindings := make(map[string][]docker.PortBinding)
		portBindings["3376/tcp"] = []docker.PortBinding{docker.PortBinding{HostIp: "0.0.0.0", HostPort: "3376"}}
		hostConfig = docker.HostConfig{
			PortBindings:  portBindings,
			RestartPolicy: docker.RestartPolicy{Name: "always"},
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
	if err := client.RemoveContainer(name, true, true); err != nil {
		log.Debugf("[containerhandler] Couldn't remove container: %s: %s", name, err)
	} else {
		log.Debugf("[containerhandler] Force removed container with name %s.", name)
	}
	containerID, createErr := client.CreateContainer(config, name)
	if createErr != nil {
		log.Errorf("[containerhandler] Failed to create Swarm manager container: %s", createErr)
		return "", createErr
	}
	log.Debugf("[containerhandler] Created swarm manager container successfully, trying to start it.  [Name: %s]", name)
	if startErr := client.StartContainer(containerID, &hostConfig); startErr != nil {
		log.Errorf("[containerhandler] Failed to start Swarm manager container: %s", startErr)
		return "", startErr
	}
	log.Infof("[containerhandler] Started swarm manager container [Name: %s, ID: %s]", name, containerID)
	return containerID, nil
}
