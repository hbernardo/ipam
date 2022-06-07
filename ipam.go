package ipam

import (
	"fmt"
	"net"
	"strings"
)

type IPAMPoolDatacenterSettings struct {
	Type             string `json:"type"`
	PoolCIDR         string `json:"poolCidr"`
	AllocationPrefix uint8  `json:"allocationPrefix,omitempty"`
	AllocationRange  uint32 `json:"allocationRange,omitempty"`
}

type IPAMAllocation struct {
	IPAMPoolName string
	Type         string   `json:"type"`
	CIDR         string   `json:"cidr,omitempty"`
	Addresses    []string `json:"addresses,omitempty"`
}

type IPAMPool struct {
	Name        string
	Datacenters map[string]IPAMPoolDatacenterSettings `json:"datacenters"`
}

type Cluster struct {
	Name            string
	IPAMAllocations []IPAMAllocation
}

type ipam struct {
	datacenterAllocations map[string][]Cluster
	dcIPAMPoolUsageMap    map[string]map[string]struct{}
}

func newIPAM(dcAllocations map[string][]Cluster) ipam {
	ipam := ipam{
		datacenterAllocations: dcAllocations,
		dcIPAMPoolUsageMap:    make(map[string]map[string]struct{}),
	}

	return ipam
}

func (p ipam) clearUsageMap() {
	for k := range p.dcIPAMPoolUsageMap {
		delete(p.dcIPAMPoolUsageMap, k)
	}
}

func (p ipam) setUsed(dc string, ipamPool string, value string) {
	key := fmt.Sprintf("%s-%s", dc, ipamPool)
	_, hasUsedIPs := p.dcIPAMPoolUsageMap[key]
	if !hasUsedIPs {
		p.dcIPAMPoolUsageMap[key] = map[string]struct{}{}
	}
	p.dcIPAMPoolUsageMap[key][value] = struct{}{}
}

func (p ipam) isUsed(dc string, ipamPool string, value string) bool {
	usedValues, hasUsedValues := p.dcIPAMPoolUsageMap[fmt.Sprintf("%s-%s", dc, ipamPool)]
	if hasUsedValues {
		_, isUsed := usedValues[value]
		return isUsed
	}
	return false
}

func (p ipam) apply(ipamPool IPAMPool) error {
	p.clearUsageMap()

	// calculate used IPs for each datacenter IPAMPool
	for dc, clusters := range p.datacenterAllocations {
		for _, cluster := range clusters {
			for _, clusterAllocation := range cluster.IPAMAllocations {
				switch clusterAllocation.Type {
				case "range":
					for _, addressRange := range clusterAllocation.Addresses {
						ipRange := strings.SplitN(addressRange, "-", 2)
						if len(ipRange) != 2 {
							return fmt.Errorf("wrong ip range format")
						}
						firstIP := net.ParseIP(ipRange[0])
						if firstIP == nil {
							return fmt.Errorf("wrong ip format")
						}
						lastIP := net.ParseIP(ipRange[1])
						if lastIP == nil {
							return fmt.Errorf("wrong ip format")
						}
						for ip := firstIP; !ip.Equal(lastIP); ip = incIP(ip) {
							p.setUsed(dc, clusterAllocation.IPAMPoolName, ip.String())
						}
						p.setUsed(dc, clusterAllocation.IPAMPoolName, lastIP.String())
					}
				case "prefix":
					if clusterAllocation.CIDR != "" {
						p.setUsed(dc, clusterAllocation.IPAMPoolName, clusterAllocation.CIDR)
					}
				}
			}
		}
	}

	for dc, dcConfig := range ipamPool.Datacenters {
		err := p.setDatacenterAllocation(dc, ipamPool.Name, dcConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p ipam) setDatacenterAllocation(dc, ipamPool string, dcConfig IPAMPoolDatacenterSettings) error {
	clusters, dcFound := p.datacenterAllocations[dc]
	if !dcFound {
		return fmt.Errorf("no cluster deployed in datacenter %s", dc)
	}

	rangeFreeIPs := []string{}
	rangeFreeIPsIterator := 0
	if dcConfig.Type == "range" {
		// calculate free ips from pool cidr
		ip, ipNet, err := net.ParseCIDR(string(dcConfig.PoolCIDR))
		if err != nil {
			return err
		}
		for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); ip = incIP(ip) {
			if p.isUsed(dc, ipamPool, ip.String()) {
				continue
			}
			rangeFreeIPs = append(rangeFreeIPs, ip.String())
		}
	}

	newClustersAllocations := make([]IPAMAllocation, len(clusters))

	// loop clusters for the new allocations
	for i, cluster := range clusters {
		// loop all current cluster allocations to check and return error if a pool with same name was already applied
		for _, curAllocation := range cluster.IPAMAllocations {
			if curAllocation.IPAMPoolName == ipamPool {
				return fmt.Errorf("pool %s is already applied to the cluster %s", ipamPool, cluster.Name)
			}
		}

		newClustersAllocations[i] = IPAMAllocation{
			IPAMPoolName: ipamPool,
			Type:         dcConfig.Type,
		}

		switch dcConfig.Type {
		case "range":
			newClustersAllocations[i].Addresses = []string{}

			quantityToAllocate := int(dcConfig.AllocationRange)
			if (rangeFreeIPsIterator + quantityToAllocate) > len(rangeFreeIPs) {
				return fmt.Errorf("there is no enough free IPs available for pool %s", ipamPool)
			}

			firstAddressRangeIP := rangeFreeIPs[rangeFreeIPsIterator]
			for j := 0; j < quantityToAllocate; j++ {
				ipToAllocate := rangeFreeIPs[rangeFreeIPsIterator]
				p.setUsed(dc, ipamPool, ipToAllocate)
				rangeFreeIPsIterator++
				// if no next ip to allocate or next ip is not the next one, close a new address range
				if j+1 == quantityToAllocate || !isTheNextIP(rangeFreeIPs[rangeFreeIPsIterator], ipToAllocate) {
					addressRange := fmt.Sprintf("%s-%s", firstAddressRangeIP, ipToAllocate)
					newClustersAllocations[i].Addresses = append(newClustersAllocations[i].Addresses, addressRange)
					if j+1 < quantityToAllocate {
						firstAddressRangeIP = rangeFreeIPs[rangeFreeIPsIterator]
					}
				}
			}
		case "prefix":
			var err error
			newClustersAllocations[i].CIDR, err = p.findFirstFreeSubnetOfPool(dc, ipamPool, string(dcConfig.PoolCIDR), int(dcConfig.AllocationPrefix))
			if err != nil {
				return err
			}

			// mark subnet as used
			p.setUsed(dc, ipamPool, newClustersAllocations[i].CIDR)
		}
	}

	// if no error happened, add the new clusters allocations
	for i, newClusterAllocation := range newClustersAllocations {
		p.datacenterAllocations[dc][i].IPAMAllocations = append(p.datacenterAllocations[dc][i].IPAMAllocations, newClusterAllocation)
	}

	return nil
}

func (p ipam) findFirstFreeSubnetOfPool(dc, ipamPool, poolCIDR string, subnetPrefix int) (string, error) {
	poolIP, poolSubnet, err := net.ParseCIDR(poolCIDR)
	if err != nil {
		return "", err
	}

	poolPrefix, bits := poolSubnet.Mask.Size()
	if subnetPrefix < poolPrefix {
		return "", fmt.Errorf("invalid prefix for subnet")
	}
	if subnetPrefix > bits {
		return "", fmt.Errorf("invalid prefix for subnet")
	}

	_, possibleSubnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", poolIP.Mask(poolSubnet.Mask), subnetPrefix))
	if err != nil {
		return "", err
	}
	for ; poolSubnet.Contains(possibleSubnet.IP); possibleSubnet, _ = nextSubnet(possibleSubnet, subnetPrefix) {
		if !p.isUsed(dc, ipamPool, possibleSubnet.String()) {
			return possibleSubnet.String(), nil
		}
	}

	return "", fmt.Errorf("cannot find free subnet")
}
