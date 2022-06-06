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
	Type      string   `json:"type"`
	CIDR      string   `json:"cidr,omitempty"`
	Addresses []string `json:"addresses,omitempty"`
}

type ipam struct {
	datacenterAllocations map[string][]IPAMAllocation
	datacenterUsedIPs     map[string]map[string]struct{}
}

func newIPAM(dcAllocations map[string][]IPAMAllocation) ipam {
	ipam := ipam{
		datacenterAllocations: dcAllocations,
		datacenterUsedIPs:     make(map[string]map[string]struct{}, len(dcAllocations)),
	}

	for dc := range dcAllocations {
		ipam.datacenterUsedIPs[dc] = map[string]struct{}{}
	}

	return ipam
}

type IPAMPoolSpec struct {
	Datacenters map[string]IPAMPoolDatacenterSettings `json:"datacenters"`
}

func (p ipam) reconcile(ipamPoolSpec IPAMPoolSpec) error {
	// calculate used IPs for each datacenter
	for dc, dcAllocations := range p.datacenterAllocations {
		for _, dcAllocation := range dcAllocations {
			switch dcAllocation.Type {
			case "range":
				for _, addressRange := range dcAllocation.Addresses {
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
						p.datacenterUsedIPs[dc][ip.String()] = struct{}{}
					}
					p.datacenterUsedIPs[dc][lastIP.String()] = struct{}{}
				}
			case "prefix":
				if string(dcAllocation.CIDR) != "" {
					ip, ipNet, err := net.ParseCIDR(string(dcAllocation.CIDR))
					if err != nil {
						return err
					}
					for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip, 1) {
						p.datacenterUsedIPs[dc][ip.String()] = struct{}{}
					}
				}
			}
		}
	}

	for dc, dcConfig := range ipamPoolSpec.Datacenters {
		err := p.setDatacenterAllocation(dc, dcConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p ipam) setDatacenterAllocation(dc string, dcConfig IPAMPoolDatacenterSettings) error {
	dcAllocations, dcFound := p.datacenterAllocations[dc]
	if !dcFound {
		return fmt.Errorf("unknown datacenter")
	}

	// calculate free ips from pool cidr
	freeIPs := []string{}

	ip, ipNet, err := net.ParseCIDR(string(dcConfig.PoolCIDR))
	if err != nil {
		return err
	}
	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip, 1) {
		_, isUsedIP := p.datacenterUsedIPs[dc][ip.String()]
		if isUsedIP {
			continue
		}
		freeIPs = append(freeIPs, ip.String())
	}

	// loop datacenter allocations for the new allocations
	freeIPsIterator := 0
	for i, dcAllocation := range dcAllocations {
		switch dcConfig.Type {
		case "range":
			// skip already allocated cluster
			if len(dcAllocation.Addresses) > 0 || dcAllocation.CIDR != "" {
				continue
			}

			quantityToAllocate := int(dcConfig.AllocationRange)

			newAdresses := []string{}
			firstAddressRangeIP := freeIPs[freeIPsIterator]
			for j := 0; j < quantityToAllocate; j++ {
				ipToAllocate := freeIPs[freeIPsIterator]
				p.datacenterUsedIPs[dc][ipToAllocate] = struct{}{}
				freeIPsIterator++
				// if no next ip to allocate or next ip is not the next one, close a new address range
				if j+1 == quantityToAllocate || !isTheNextIP(freeIPs[freeIPsIterator], ipToAllocate) {
					addressRange := fmt.Sprintf("%s-%s", firstAddressRangeIP, ipToAllocate)
					newAdresses = append(newAdresses, addressRange)
					if j+1 < quantityToAllocate {
						firstAddressRangeIP = freeIPs[freeIPsIterator]
					}
				}
			}
			p.datacenterAllocations[dc][i].Type = "range"
			p.datacenterAllocations[dc][i].Addresses = append(dcAllocation.Addresses, newAdresses...)
		case "prefix":
			// skip already allocated cluster
			if dcAllocation.CIDR != "" || len(dcAllocation.Addresses) > 0 {
				continue
			}

			subnetCIDR, err := p.findFirstFreeSubnetOfPool(dc, string(dcConfig.PoolCIDR), int(dcConfig.AllocationPrefix))
			if err != nil {
				return err
			}
			// mark all subnet IPs as used
			ip, ipNet, err := net.ParseCIDR(string(subnetCIDR))
			if err != nil {
				return err
			}
			for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip, 1) {
				p.datacenterUsedIPs[dc][ip.String()] = struct{}{}
			}
			p.datacenterAllocations[dc][i].Type = "prefix"
			p.datacenterAllocations[dc][i].CIDR = subnetCIDR
		}
	}

	return nil
}

func (p ipam) findFirstFreeSubnetOfPool(dc string, poolCIDR string, subnetPrefix int) (string, error) {
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
			_, isUsedIP := p.datacenterUsedIPs[dc][sIP.String()]
			if isUsedIP {
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
