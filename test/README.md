## Testing


Testing with local CentOS 7 cluster
```
cd centos7-cluster
vagrant plugin install vagrant-cachier
vagrant up
vagrant ssh node0

#Inside the VM
cd /swarm-bootstrap && sudo docker build --rm -t munchausen .
sudo docker run -it -v /var/run/docker.sock:/var/run/docker.sock munchausen --debug bootstrap --consulServers=10.10.38.10 10.10.38.10:2376,10.10.38.11:2376,10.10.38.12:2376
sudo docker run -it -v /var/run/docker.sock:/var/run/docker.sock munchausen --debug add --join=consul://10.10.38.10:8500 10.10.38.13:2376
docker -H tcp://127.0.0.1:3376 info
```
