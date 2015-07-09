package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"github.com/codegangsta/cli"
	consul "github.com/hashicorp/consul/api"
	docker "github.com/samalba/dockerclient"
	log "github.com/Sirupsen/logrus"
)

const (
	MaxGetLeaderAttempts                 = 12
	SecondsBetweenGetLeaderAttempts      = 5
	SecondsBetweenDockerPings            = 5
	MaxGetSwarmAgentsAttempts            = 12
	SecondsBetweenGetSwarmAgentsAttempts = 5
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
	wait := c.Int(flWait.Name)
	consulLogLocation := c.String(flConsulLogLocation.Name)
	var fallbackDNSRecursors []string
	if (len(c.String(flFallbackDNSRecursors.Name)) > 0) {
		fallbackDNSRecursors = strings.Split(c.String(flFallbackDNSRecursors.Name), ",")
	}

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

	log.Infof("[bootstrap] Started Swarm bootstrapping with parameters. Consul servers: %v, Nodes: %s, Wait: %v", len(consulServers), nodesAsString, wait)

	if wait > 0 {
		nodes = waitForDockerDaemons(wait, nodes)
		log.Infof("[bootstrap] Continuing with %v nodes: %s", len(nodes), nodes)
	}
	bootstrapNewNodes(nodesAsString, consulServers, nodes, consulLogLocation, true, fallbackDNSRecursors)

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
	wait := c.Int(flWait.Name)
	consulLogLocation := c.String(flConsulLogLocation.Name)
	var fallbackDNSRecursors []string
	if (len(c.String(flFallbackDNSRecursors.Name)) > 0) {
		fallbackDNSRecursors = strings.Split(c.String(flFallbackDNSRecursors.Name), ",")
	}

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

	log.Infof("[bootstrap] Started Swarm add with parameters. Consul servers: %v, Nodes: %s, Wait: %v", len(peers), nodesAsString, wait)

	if wait > 0 {
		nodes = waitForDockerDaemons(wait, nodes)
		log.Infof("[bootstrap] Continuing with %v nodes: %s", len(nodes), nodes)
	}
	bootstrapNewNodes(nodesAsString, peers, nodes, consulLogLocation, false, fallbackDNSRecursors)

	log.Info("[bootstrap] Finished adding new nodes to the Consul-Swarm cluster.")
}

func waitForDockerDaemons(wait int, nodes []string) []string {
	var maxDockerPingAttemps int = wait/SecondsBetweenDockerPings + 1
	log.Infof("[bootstrap] Trying to reach docker daemons in %v attempts.", maxDockerPingAttemps)
	var nodesFound []string
	for i := 0; ; i++ {
		if i >= maxDockerPingAttemps {
			log.Infof("[bootstrap] Max attempts were reached, %v docker daemons are available.", len(nodesFound))
			return nodesFound
		}
		if len(nodesFound) == len(nodes) {
			log.Infof("[bootstrap] All docker daemons are available.")
			return nodesFound
		}
		time.Sleep(SecondsBetweenDockerPings * time.Second)
		log.Debugf("[bootstrap] Ping docker daemons, attempt %v", i+1)
		var wg sync.WaitGroup
		for _, node := range nodes {
			wg.Add(1)
			go func(node string) {
				defer wg.Done()
				time.Sleep(1 * time.Second)
				if contains(nodesFound, node) {
					log.Debugf("[bootstrap] Docker daemon on node %s has already responded, not pinging it again.", node)
					return
				}
				client, err := docker.NewDockerClientTimeout("http://"+node, nil, time.Duration(5*time.Second))
				if err != nil {
					log.Debugf("[bootstrap] Couldn't create client for docker daemon (%s): %s", node, err)
					return
				}
				if _, err := client.Info(); err != nil {
					log.Debugf("[bootstrap] Failed to retrieve info from docker daemon (%s): %s", node, err)
				} else {
					log.Debugf("[bootstrap] Docker daemon responded on node %s.", node)
					nodesFound = append(nodesFound, node)
				}
			}(node)
		}
		wg.Wait()
		log.Infof("[bootstrap] Attempt %v: %v docker daemons are available.", i+1, len(nodesFound))
	}
}

func bootstrapNewNodes(nodesAsString string, consulServers []string, nodes []string, consulLogLocation string, startSwarmManager bool, fallbackDNSRecursors []string) {
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

	log.Debug("[bootstrap] Wait for temporary swarm manager to find the nodes.")
	time.Sleep(SecondsBetweenGetSwarmAgentsAttempts * time.Second)

	log.Debug("[bootstrap] Creating docker client for temporary Swarm Manager.")
	tmpSwarmClient, err := docker.NewDockerClient("http://" + tmpSwarmManagerContainer.NetworkSettings.IPAddress + ":3376", nil)
	if err != nil {
		log.Fatalf("[bootstrap] Failed to create Docker client for temporary Swarm manager: %s", err)
	}

	var swarmNodes []*SwarmNode
	for i := 0; ; i++ {
		if i >= MaxGetSwarmAgentsAttempts {
			log.Infof("[bootstrap] Failed to get all Swarm nodes in %v attempts.", MaxGetSwarmAgentsAttempts)
			break
		}
		log.Debugf("[bootstrap] Retrieving nodes from temporary Swarm manager, attempt %v", i)
		var swarmNodesErr error
		swarmNodes, swarmNodesErr = getSwarmNodes(tmpSwarmClient)
		if swarmNodesErr != nil {
			log.Debugf("[bootstrap] Failed to get nodes from temporary Swarm manager: %s", err)
		} else if len(swarmNodes) == len(nodes) {
			log.Info("[bootstrap] Found all Swarm nodes.")
			break
		}
		time.Sleep(SecondsBetweenGetSwarmAgentsAttempts * time.Second)
	}

	if len(swarmNodes) != len(nodes) {
		log.Warnf("[bootstrap] Swarm manager found %v nodes but expected %v. It is possible that some of the nodes won't be joined to the cluster.", len(swarmNodes), len(nodes))
	}

	var wg sync.WaitGroup
	for _, node := range swarmNodes {
		wg.Add(1)
		go func(node *SwarmNode) {
			defer wg.Done()
			runConsulConfigCopyContainer(tmpSwarmClient, "copy", node, consulServers, consulLogLocation, fallbackDNSRecursors)
		}(node)
	}

	log.Debug("[bootstrap] Wait for consul configurations to be ready on all nodes.")
	wg.Wait()

	for _, node := range swarmNodes {
		wg.Add(1)
		go func(node *SwarmNode) {
			defer wg.Done()
			runConsulContainer(tmpSwarmClient, "consul", node, consulLogLocation)
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
		go func(node *SwarmNode) {
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

func contains(stringSlice []string, element string) bool {
	for _, s := range stringSlice {
		if s == element {
			return true
		}
	}
	return false
}
