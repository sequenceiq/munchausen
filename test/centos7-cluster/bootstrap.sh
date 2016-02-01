#!/bin/bash

: ${IMAGES:=sequenceiq/consul:v0.5.0-v5 swarm:0.3.0}
: ${DEBUG:=1}

debug() {
    [[ "$DEBUG" ]] && echo "-----> $*" 1>&2
}

install_utils() {
  yum -y install deltarpm unzip curl git wget bind-utils ntp golang tee
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
  tee /etc/yum.repos.d/docker.repo <<-'EOF'
[dockerrepo]
name=Docker Repository
baseurl=https://yum.dockerproject.org/repo/main/centos/$releasever/
enabled=1
gpgcheck=1
gpgkey=https://yum.dockerproject.org/gpg
EOF
  yum -y install docker-engine
  sed -i '/^ExecStart/s/$/ -H tcp:\/\/0.0.0.0:2376 --selinux-enabled=false --storage-driver=overlay/' /usr/lib/systemd/system/docker.service
  systemctl daemon-reload
  systemctl start docker
  systemctl enable docker
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
