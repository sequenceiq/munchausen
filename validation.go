package main

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

func validateAddress(address string) error {
	parsedAddress := strings.Split(address, ":")
	if len(parsedAddress) == 2 {
		if net.ParseIP(parsedAddress[0]) == nil {
			return fmt.Errorf("%s is not a valid address.", address)
		}
		if _, err := strconv.Atoi(parsedAddress[1]); err != nil {
			return fmt.Errorf("%s is not a valid address.", address)
		}
	} else {
		return fmt.Errorf("%s is not in address:port format.", address)
	}
	return nil
}

func validateNodeUris(nodesArg string) ([]string, error) {
	var nodes []string
	if strings.HasPrefix(nodesArg, "nodes://") {
		nodesArg = nodesArg[8:]
	}
	for _, node := range strings.Split(nodesArg, ",") {
		re, _ := regexp.Compile(`\[(\d+):(\d+)\]`)
		submatches := re.FindAllStringSubmatch(node, -1)
		switch {
		case len(submatches) < 1:
			if err := validateAddress(node); err != nil {
				return nil, err
			}
			nodes = append(nodes, node)
		case len(submatches) > 1:
			return nil, fmt.Errorf("More than one range expression in address %s", node)
		case len(submatches) == 1:
			submatch := submatches[0]
			from, _ := strconv.Atoi(submatch[1])
			to, _ := strconv.Atoi(submatch[2])
			template := re.ReplaceAllString(node, "%d")
			fromAddr := fmt.Sprintf(template, from)
			toAddr := fmt.Sprintf(template, to)
			if err := validateAddress(fromAddr); err != nil {
				return nil, err
			}
			if err := validateAddress(toAddr); err != nil {
				return nil, err
			}
			for val := from; val <= to; val++ {
				entry := fmt.Sprintf(template, val)
				nodes = append(nodes, entry)
			}
		}
	}
	return nodes, nil
}

func validateConsulServerAddresses(consulServers []string) error {
	for _, serverAddress := range consulServers {
		if net.ParseIP(serverAddress) == nil {
			return fmt.Errorf("[validation] Consul server address %s is not a valid IP address", serverAddress)
		}
	}
	return nil
}
