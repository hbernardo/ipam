package ipam

import (
	"fmt"
	"math"
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
	dcIPAMPoolUsedIPs     map[string]map[string]struct{}
}

func newIPAM(dcAllocations map[string][]Cluster) ipam {
	ipam := ipam{
		datacenterAllocations: dcAllocations,
		dcIPAMPoolUsedIPs:     make(map[string]map[string]struct{}),
	}

	return ipam
}

func (p ipam) resetUsedIPs() {
	for k := range p.dcIPAMPoolUsedIPs {
		delete(p.dcIPAMPoolUsedIPs, k)
	}
}

func (p ipam) setUsedIP(dc string, ipamPool string, ip string) {
	key := fmt.Sprintf("%s-%s", dc, ipamPool)
	_, hasUsedIPs := p.dcIPAMPoolUsedIPs[key]
	if !hasUsedIPs {
		p.dcIPAMPoolUsedIPs[key] = map[string]struct{}{}
	}
	p.dcIPAMPoolUsedIPs[key][ip] = struct{}{}
}

func (p ipam) isUsedIP(dc string, ipamPool string, ip string) bool {
	usedIPs, hasUsedIPs := p.dcIPAMPoolUsedIPs[fmt.Sprintf("%s-%s", dc, ipamPool)]
	if hasUsedIPs {
		_, isUsedIP := usedIPs[ip]
		return isUsedIP
	}
	return false
}

func (p ipam) apply(ipamPool IPAMPool) error {
	p.resetUsedIPs()

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
						for ip := firstIP; !ip.Equal(lastIP); incIP(ip, 1) {
							p.setUsedIP(dc, clusterAllocation.IPAMPoolName, ip.String())
						}
						p.setUsedIP(dc, clusterAllocation.IPAMPoolName, lastIP.String())
					}
				case "prefix":
					if string(clusterAllocation.CIDR) != "" {
						ip, ipNet, err := net.ParseCIDR(string(clusterAllocation.CIDR))
						if err != nil {
							return err
						}
						for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip, 1) {
							p.setUsedIP(dc, clusterAllocation.IPAMPoolName, ip.String())
						}
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

	// calculate free ips from pool cidr
	freeIPs := []string{}

	ip, ipNet, err := net.ParseCIDR(string(dcConfig.PoolCIDR))
	if err != nil {
		return err
	}
	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip, 1) {
		if p.isUsedIP(dc, ipamPool, ip.String()) {
			continue
		}
		freeIPs = append(freeIPs, ip.String())
	}

	newClustersAllocations := make([]IPAMAllocation, len(clusters))

	// loop clusters for the new allocations
	freeIPsIterator := 0
	for i, cluster := range clusters {
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
			if (freeIPsIterator + quantityToAllocate) > len(freeIPs) {
				return fmt.Errorf("there is no enough free IPs available for pool %s", ipamPool)
			}

			firstAddressRangeIP := freeIPs[freeIPsIterator]
			for j := 0; j < quantityToAllocate; j++ {
				ipToAllocate := freeIPs[freeIPsIterator]
				p.setUsedIP(dc, ipamPool, ipToAllocate)
				freeIPsIterator++
				// if no next ip to allocate or next ip is not the next one, close a new address range
				if j+1 == quantityToAllocate || !isTheNextIP(freeIPs[freeIPsIterator], ipToAllocate) {
					addressRange := fmt.Sprintf("%s-%s", firstAddressRangeIP, ipToAllocate)
					newClustersAllocations[i].Addresses = append(newClustersAllocations[i].Addresses, addressRange)
					if j+1 < quantityToAllocate {
						firstAddressRangeIP = freeIPs[freeIPsIterator]
					}
				}
			}
		case "prefix":
			newClustersAllocations[i].CIDR, err = p.findFirstFreeSubnetOfPool(dc, ipamPool, string(dcConfig.PoolCIDR), int(dcConfig.AllocationPrefix))
			if err != nil {
				return err
			}

			// mark all subnet IPs as used
			ip, ipNet, err := net.ParseCIDR(string(newClustersAllocations[i].CIDR))
			if err != nil {
				return err
			}
			for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip, 1) {
				p.setUsedIP(dc, ipamPool, ip.String())
			}
		}
	}

	// if no error happened, add the new clusters allocations
	for i, newClusterAllocation := range newClustersAllocations {
		p.datacenterAllocations[dc][i].IPAMAllocations = append(p.datacenterAllocations[dc][i].IPAMAllocations, newClusterAllocation)
	}

	return nil
}

func (p ipam) findFirstFreeSubnetOfPool(dc, ipamPool, poolCIDR string, subnetPrefix int) (string, error) {
	ip, ipNet, err := net.ParseCIDR(poolCIDR)
	if err != nil {
		return "", err
	}

	poolPrefix, bits := ipNet.Mask.Size()
	if subnetPrefix < poolPrefix {
		return "", fmt.Errorf("invalid prefix for subnet")
	}
	if subnetPrefix > bits {
		return "", fmt.Errorf("invalid prefix for subnet")
	}
	subnetSize := int(math.Pow(2, float64(bits-subnetPrefix)))

	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip, subnetSize) {
		sIP, possibleSubnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip, subnetPrefix))
		if err != nil {
			return "", err
		}
		subnetIsOccupied := false
		for sIP := sIP.Mask(possibleSubnet.Mask); possibleSubnet.Contains(sIP); incIP(sIP, 1) {
			if p.isUsedIP(dc, ipamPool, sIP.String()) {
				subnetIsOccupied = true
				break
			}
		}
		if !subnetIsOccupied {
			return possibleSubnet.String(), nil
		}
	}

	return "", fmt.Errorf("cannot find free subnet")
}

func incIP(ip net.IP, count int) {
	for i := 0; i < count; i++ {
		for j := len(ip) - 1; j >= 0; j-- {
			ip[j]++
			if ip[j] > 0 {
				break
			}
		}
	}
}

func isTheNextIP(ipToCheck string, previousIP string) bool {
	nextIP := net.ParseIP(previousIP)
	incIP(nextIP, 1)
	return nextIP.Equal(net.ParseIP(ipToCheck))
}
