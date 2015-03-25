package main

import (
	"flag"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"log"
	"time"
)

type empty struct{}

func main() {

	log.Println("[main] Launched swarm-bootstrap application")

	var consulServerCount int
	flag.IntVar(&consulServerCount, "consulServers", 3, "the number of Consul servers")

	flag.Parse()

	log.Println("Consul servers:", consulServerCount)

	log.Println("Nodes:", flag.Args())

	data := make([]float32, 4)

	sem := make(chan empty, len(data))
	for i, _ := range data {
		go func(i int) {
			data[i] += 5
			time.Sleep(1000 * time.Millisecond)
			log.Println(i, data[i])
			sem <- empty{}
		}(i)
	}
	for i := 0; i < len(data); i++ {
		<-sem
	}

	endpoint := "unix:///var/run/docker.sock"
	client, _ := docker.NewClient(endpoint)

	imgs, _ := client.ListImages(docker.ListImagesOptions{All: false})

	for _, img := range imgs {
		fmt.Println("ID: ", img.ID)
		fmt.Println("RepoTags: ", img.RepoTags)
	}

	log.Println("siker")
}
