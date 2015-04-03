package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	docker "github.com/martonsereg/dockerclient"
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
