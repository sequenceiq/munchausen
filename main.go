package main

import (
	"flag"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"log"
	"strings"
	"time"
)

type empty struct{}

func main() {

	log.Println("[main] Launched swarm-bootstrap application")

	var consulServerCount int
	flag.IntVar(&consulServerCount, "consulServers", 3, "the number of Consul servers")
	flag.Parse()
	var nodes string = strings.Join(flag.Args(), ",")

	log.Println("Consul servers:", consulServerCount)
	log.Println("Nodes:", nodes)

	// data := make([]float32, 4)

	// sem := make(chan empty, len(data))
	// for i, _ := range data {
	// 	go func(i int) {
	// 		data[i] += 5
	// 		time.Sleep(1000 * time.Millisecond)
	// 		log.Println(i, data[i])
	// 		sem <- empty{}
	// 	}(i)
	// }
	// for i := 0; i < len(data); i++ {
	// 	<-sem
	// }

	endpoint := "unix:///var/run/docker.sock"
	client, _ := docker.NewClient(endpoint)

	config := docker.Config{
		Image: "swarm",
		Cmd:   []string{"manage", "-H", "tcp://0.0.0.0:3376", nodes},
	}
	container, _ := client.CreateContainer(docker.CreateContainerOptions{Name: "tmp-swarm-mgr", Config: &config})
	client.StartContainer(container.ID, &docker.HostConfig{})

	container, _ = client.InspectContainer(container.ID)
	log.Println("Started temporary Swarm manager.", container.ID, container.NetworkSettings.IPAddress)

	log.Println("http://" + container.NetworkSettings.IPAddress + ":3376")
	tmpSwarmClient, _ := docker.NewClient("http://" + container.NetworkSettings.IPAddress + ":3376")

	log.Println("Wait for swarm manager.")
	time.Sleep(3000 * time.Millisecond)

	info, _ := tmpSwarmClient.Info()

	imgs, _ := tmpSwarmClient.ListImages(docker.ListImagesOptions{All: false})
	for _, img := range imgs {
		fmt.Println("ID: ", img.ID)
		fmt.Println("RepoTags: ", img.RepoTags)
	}

	log.Println(info)

}
