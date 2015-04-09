package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	consul "github.com/hashicorp/consul/api"
	docker "github.com/martonsereg/dockerclient"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	MaxGetLeaderAttempts            = 12
	SecondsBetweenGetLeaderAttempts = 5
)

func bootstrap(c *cli.Context) {

	if len(c.Args()) != 1 {
		log.Fatalf("[bootstrap] Nodes must be provided as a comma separated list without whitespace. See '%s bootstrap --help'.", c.App.Name)
	}

	if len(c.String(flConsulServers.Name)) == 0 {
		log.Fatalf("[bootstrap] --consulServers is required. See '%s bootstrap --help'.", c.App.Name)
	}

	nodesAsString := c.Args()[0]
	consulServers := strings.Split(c.String(flConsulServers.Name), ",")

	nodes, err := validateNodeUris(nodesAsString)
	if err != nil {
		log.Fatal(err)
	}

	if err := validateConsulServerAddresses(consulServers); err != nil {
		log.Fatal(err)
	}

	for _, serverAddress := range consulServers {
		valid := false
		for _, node := range nodes {
			if strings.Contains(node, serverAddress) {
				valid = true
				break
			}
		}
		if !valid {
			log.Fatalf("[bootstrap] Consul server %s is not among the provided nodes", serverAddress)
		}
	}

	if len(consulServers) != 3 && len(consulServers) != 5 && len(consulServers) != 7 {
		log.Warnf("[bootstrap] %v consul servers configured. Recommended is 3 or 5.", len(consulServers))
	}

	log.Infof("[bootstrap] Started Swarm bootstrapping with parameters. Consul servers: %v, Nodes: %s", len(consulServers), nodesAsString)

	bootstrapNewNodes(nodesAsString, consulServers, nodes, true)

	log.Info("[bootstrap] Finished Swarm bootstrapping.")
	log.Info("[bootstrap] To try the new Swarm Manager run 'docker -H tcp://127.0.0.1:3376 info'")
}

func add(c *cli.Context) {

	if len(c.Args()) != 1 {
		log.Fatalf("[bootstrap] Nodes must be provided as a comma separated list. See '%s add --help'.", c.App.Name)
	}

	if len(c.String(flConsulJoin.Name)) == 0 {
		log.Fatalf("[bootstrap] --join is required. See '%s bootstrap --help'.", c.App.Name)
	}

	nodesAsString := c.Args()[0]

	consulJoin := c.String(flConsulJoin.Name)
	if strings.HasPrefix(consulJoin, "consul://") {
		consulJoin = consulJoin[9:]
	}
	if err := validateAddress(consulJoin); err != nil {
		log.Fatal(err)
	}

	nodes, err := validateNodeUris(nodesAsString)
	if err != nil {
		log.Fatal(err)
	}

	peers, err := getConsulPeers(consulJoin)
	if err != nil {
		log.Fatal(err)
	}
	// TODO: check if consul-servers are new nodes or not, check if other nodes are not in the cluster already

	log.Infof("[bootstrap] Started Swarm add with parameters. Consul servers: %v, Nodes: %s", len(peers), nodesAsString)

	bootstrapNewNodes(nodesAsString, peers, nodes, false)

	log.Info("[bootstrap] Finished adding new nodes to the Consul-Swarm cluster.")
}

func bootstrapNewNodes(nodesAsString string, consulServers []string, nodes []string, startSwarmManager bool) {
	log.Debug("[bootstrap] Creating docker client with docker.sock.")
	client, err := docker.NewDockerClient("unix:///var/run/docker.sock", nil)
	if err != nil {
		log.Fatalf("[bootstrap] Failed to create Docker client with docker.sock: %s", err)
	}

	tmpSwarmManagerID, swarmManagerErr := runSwarmManagerContainer(client, TmpSwarmContainerName, nodesAsString, false)
	if swarmManagerErr != nil {
		os.Exit(1)
	}

	tmpSwarmManagerContainer, inspectErr := client.InspectContainer(tmpSwarmManagerID)
	if inspectErr != nil {
		log.Fatalf("[bootstrap] Failed to inspect temporary Swarm manager container: %s", err)
	}
	log.Debug("[bootstrap] Wait 3 seconds for swarm manager.")
	time.Sleep(3000 * time.Millisecond)
	log.Debug("[bootstrap] Creating docker client for temporary Swarm Manager.")
	tmpSwarmClient, err := docker.NewDockerClient("http://"+tmpSwarmManagerContainer.NetworkSettings.IpAddress+":3376", nil)
	if err != nil {
		log.Fatalf("[bootstrap] Failed to create Docker client for temporary Swarm manager: %s", err)
	}

	swarmNodes, err := getSwarmNodes(tmpSwarmClient)
	if err != nil {
		log.Fatal(err)
	}
	if len(swarmNodes) != len(nodes) {
		log.Warnf("[bootstrap] Swarm manager found %v nodes but expected %v. It is possible that some of the nodes won't be joined to the cluster.", len(nodes), len(swarmNodes))
	}

	var wg sync.WaitGroup
	for _, node := range swarmNodes {
		wg.Add(1)
		go func(node *docker.SwarmNode) {
			defer wg.Done()
			runConsulConfigCopyContainer(tmpSwarmClient, "copy", node, consulServers)
		}(node)
	}

	log.Debug("[bootstrap] Wait for consul configurations to be ready on all nodes.")
	wg.Wait()

	for _, node := range swarmNodes {
		wg.Add(1)
		go func(node *docker.SwarmNode) {
			defer wg.Done()
			runConsulContainer(tmpSwarmClient, "consul", node)
		}(node)
	}

	log.Debug("[bootstrap] Wait for Consul containers to create on all nodes.")
	wg.Wait()

	peers, err := getConsulPeers(consulServers[0] + ":8500")
	if err != nil {
		log.Fatal(err)
	}
	for _, server := range consulServers {
		var found bool
		for _, peer := range peers {
			if server == peer {
				found = true
				break
			}
		}
		if !found {
			log.Fatalf("[bootstrap] Provided consul server address %s is not among the consul peers: %s.", server, peers)
		}
	}

	for _, node := range swarmNodes {
		wg.Add(1)
		go func(node *docker.SwarmNode) {
			defer wg.Done()
			runSwarmAgentContainer(tmpSwarmClient, "swarm-agent", node, consulServers[0])
		}(node)
	}

	log.Debug("[bootstrap] Wait for Swarm Agent containers to create on all nodes.")
	wg.Wait()

	if startSwarmManager {
		time.Sleep(3000 * time.Millisecond)
		if _, err := runSwarmManagerContainer(client, SwarmContainerName, "consul://"+consulServers[0]+":8500/swarm", true); err != nil {
			os.Exit(1)
		}
	}

	log.Debug("[bootstrap] Removing temporary Swarm manager.")
	client.RemoveContainer(tmpSwarmManagerContainer.Id, true, true)
	log.Info("[bootstrap] Removed temporary Swarm manager.")
}

func getConsulPeers(consulAddress string) ([]string, error) {
	log.Debug("[bootstrap] Getting Consul leader.")
	consulConfig := consul.DefaultConfig()
	consulConfig.Address = consulAddress
	consulClient, _ := consul.NewClient(consulConfig)
	for i := 0; ; i++ {
		if i >= MaxGetLeaderAttempts {
			return nil, fmt.Errorf("[bootstrap] Failed to get Consul leader in %v attempts, exiting.", MaxGetLeaderAttempts)
		}
		log.Debugf("[bootstrap] Getting consul leader, attempt %v", i)
		if leader, err := consulClient.Status().Leader(); err != nil || len(leader) == 0 {
			log.Debugf("[bootstrap] Failed to get Consul leader: %s", err)
		} else {
			log.Infof("[bootstrap] Consul leader found: %s", leader)
			break
		}
		time.Sleep(SecondsBetweenGetLeaderAttempts * time.Second)
	}

	log.Debug("[bootstrap] Getting Consul peers.")
	if peers, err := consulClient.Status().Peers(); err != nil {
		return nil, fmt.Errorf("[bootstrap] Failed to get Consul peers, exiting: %s", err)
	} else {
		for i := 0; i < len(peers); i++ {
			peers[i] = strings.Split(peers[i], ":")[0]
		}
		log.Infof("[bootstrap] Consul peers found: %s", peers)
		return peers, nil
	}
}
