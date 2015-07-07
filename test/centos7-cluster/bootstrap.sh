#!/bin/bash

: ${IMAGES:=sequenceiq/consul:v0.5.0-v5 swarm:0.3.0}
: ${DEBUG:=1}

debug() {
    [[ "$DEBUG" ]] && echo "-----> $*" 1>&2
}

install_utils() {
  yum -y install deltarpm unzip curl git wget bind-utils ntp golang
}

permissive_iptables() {
  # need to install iptables-services, othervise the 'iptables save' command will not be available
  yum -y install iptables-services net-tools

  iptables --flush INPUT
  iptables --flush FORWARD
  service iptables save
}

permissive_selinux() {
  sed -i 's/SELINUX=enforcing/SELINUX=permissive/g' /etc/selinux/config
  setenforce 0
}

install_docker() {
  curl -O -sSL https://get.docker.com/rpm/1.7.0/centos-7/RPMS/x86_64/docker-engine-1.7.0-1.el7.centos.x86_64.rpm
  yum -y localinstall --nogpgcheck docker-engine-1.7.0-1.el7.centos.x86_64.rpm
  # need to check whether we really need these (GCP / OpenStack we don't)
  yum install -y device-mapper-event-libs device-mapper-event device-mapper-event-devel
  service docker start
  service docker stop
  sed -i '/^ExecStart/s/$/ -H tcp:\/\/0.0.0.0:2376 --selinux-enabled --storage-driver=devicemapper --storage-opt=dm.basesize=30G/' /usr/lib/systemd/system/docker.service
  rm -rf /var/lib/docker
  systemctl daemon-reload
  service docker start
  systemctl enable docker.service
}

pull_images() {
  set -e
  for i in ${IMAGES}; do
    docker pull ${i}
  done
}


main() {
    install_utils
    permissive_selinux
    permissive_iptables
    install_docker
    pull_images
}

[[ "$0" == "$BASH_SOURCE" ]] && main "$@"
