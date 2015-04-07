package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	docker "github.com/martonsereg/dockerclient"
	"strings"
	"sync"
	"time"
)

func bootstrap(c *cli.Context) {

	if len(c.Args()) != 1 {
		log.Fatalf("[bootstrap] Nodes must be provided as a comma separated list without whitespace. See '%s bootstrap --help'.", c.App.Name)
	}

	nodesAsString := c.Args()[0]
	consulServers := strings.Split(c.String(flConsulServers.Name), ",")

	//TODO: check if consul server IPs are valid ip addresses, and are among the nodes
	// check if nr of consul servers are 1/3/5/7, send warnings if not

	log.Infof("[bootstrap] Started Swarm bootstrapping with parameters. Consul servers: %v, Nodes: %s", len(consulServers), nodesAsString)

	bootstrapNewNodes(nodesAsString, consulServers, true)

	log.Info("[bootstrap] Finished Swarm bootstrapping.")
	log.Info("[bootstrap] To try the new Swarm Manager run 'docker -H tcp://127.0.0.1:3376 info'")
}

func add(c *cli.Context) {

	if len(c.Args()) != 1 {
		log.Fatalf("[bootstrap] Nodes must be provided as a comma separated list. See '%s add --help'.", c.App.Name)
	}

	nodesAsString := c.Args()[0]
	consulServers := strings.Split(c.String(flConsulServers.Name), ",")

	// TODO: provided consul servers can be
	//	- already in the cluster (best case)
	//  - newly added (if someone wants new consul servers along with the additional nodes)
	//		-> error for now because adding new servers would mean different configurations
	//	- not in the cluster, not among the nodes
	//		-> error
	// TODO: check if consul-servers are already in the cluster
	// TODO: check if consul-servers are new nodes or not, check if other nodes are not in the cluster already
	// needs to set the join addresses

	log.Infof("[bootstrap] Started Swarm add with parameters. Consul servers: %v, Nodes: %s", len(consulServers), nodesAsString)

	bootstrapNewNodes(nodesAsString, consulServers, false)

	log.Info("[bootstrap] Finished adding new nodes to the Consul-Swarm cluster.")
}

func bootstrapNewNodes(nodesAsString string, consulServers []string, startSwarmManager bool) {
	log.Debug("[bootstrap] Creating docker client with docker.sock.")
	client, err := docker.NewDockerClient("unix:///var/run/docker.sock", nil)
	if err != nil {
		log.Fatal(err)
	}

	tmpSwarmManagerID := startSwarmManagerContainer(client, TmpSwarmContainerName, nodesAsString, false)
	tmpSwarmManagerContainer, _ := client.InspectContainer(tmpSwarmManagerID)
	log.Debug("[bootstrap] Wait 3 seconds for swarm manager.")
	time.Sleep(3000 * time.Millisecond)
	log.Debug("[bootstrap] Creating docker client for temporary Swarm Manager.")
	tmpSwarmClient, _ := docker.NewDockerClient("http://"+tmpSwarmManagerContainer.NetworkSettings.IpAddress+":3376", nil)

	swarmNodes := getSwarmNodes(tmpSwarmClient)

	var wg sync.WaitGroup
	for _, node := range swarmNodes {
		wg.Add(1)
		go func(node *docker.SwarmNode) {
			defer wg.Done()
			copyConsulConfigs(tmpSwarmClient, node, consulServers)
		}(node)
	}

	log.Debug("[bootstrap] Wait for consul configurations to be ready on all nodes.")
	wg.Wait()

	for i, _ := range swarmNodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			startConsulContainer(tmpSwarmClient, fmt.Sprintf("consul-%v", i))
		}(i)
	}

	log.Debug("[bootstrap] Wait for Consul containers to create on all nodes.")
	wg.Wait()

	//TODO: wait until a leader is elected, or start swarm agents with restart policy=always

	for i, node := range swarmNodes {
		wg.Add(1)
		go func(i int, node *docker.SwarmNode) {
			defer wg.Done()
			startSwarmAgentContainer(tmpSwarmClient, fmt.Sprintf("swarm-%v", i), node, consulServers[0])
		}(i, node)
	}

	log.Debug("[bootstrap] Wait for Swarm Agent containers to create on all nodes.")
	wg.Wait()

	if startSwarmManager {
		time.Sleep(3000 * time.Millisecond)
		startSwarmManagerContainer(tmpSwarmClient, SwarmContainerName, "consul://"+consulServers[0]+":8500/swarm", true)
	}

	log.Debug("[bootstrap] Removing temporary Swarm manager.")
	client.RemoveContainer(tmpSwarmManagerContainer.Id, true, true)
	log.Info("[bootstrap] Removed temporary Swarm manager.")
}
