package ipam

type IPAMPoolDatacenterSettings struct {
	Type             string `json:"type"`
	PoolCIDR         string `json:"poolCidr"`
	AllocationPrefix uint8  `json:"allocationPrefix,omitempty"`
	AllocationRange  uint32 `json:"allocationRange,omitempty"`
}

type IPAMAllocation struct {
	IPAMPoolName string
	Cluster      string
	Datacenter   string
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
}

func newIPAM(dcAllocations map[string][]Cluster) ipam {
	return ipam{
		datacenterAllocations: dcAllocations,
	}
}

func (p ipam) apply(ipamPool IPAMPool) error {
	dcIPAMPoolUsageMap, err := p.compileCurrentAllocationsForPool(ipamPool)
	if err != nil {
		return err
	}

	newClustersAllocations, err := p.generateNewAllocationsForPool(ipamPool, dcIPAMPoolUsageMap)
	if err != nil {
		return err
	}

	// add the new clusters allocations
	for _, newClusterAllocation := range newClustersAllocations {
		dcClusters := p.datacenterAllocations[newClusterAllocation.Datacenter]
		for i, dcCluster := range dcClusters {
			if dcCluster.Name == newClusterAllocation.Cluster {
				p.datacenterAllocations[newClusterAllocation.Datacenter][i].IPAMAllocations = append(p.datacenterAllocations[newClusterAllocation.Datacenter][i].IPAMAllocations, newClusterAllocation)
				break
			}
		}
	}

	return nil
}

func (p ipam) compileCurrentAllocationsForPool(ipamPool IPAMPool) (datacenterIPAMPoolUsageMap, error) {
	dcIPAMPoolUsageMap := newDatacenterIPAMPoolUsageMap()

	// Iterate current IPAM allocations to build a map of used IPs (for range allocation type)
	// or used subnets (for prefix allocation type) per datacenter pool
	for _, dcClusters := range p.datacenterAllocations {
		for _, dcCluster := range dcClusters {
			for _, ipamAllocation := range dcCluster.IPAMAllocations {
				dcIPAMPoolCfg, isDCConfigured := ipamPool.Datacenters[ipamAllocation.Datacenter]
				if !isDCConfigured || ipamAllocation.IPAMPoolName != ipamPool.Name {
					// IPAM Pool + Datacenter is not configured in the IPAM pool spec, so we can skip it
					continue
				}

				switch ipamAllocation.Type {
				case "range":
					currentAllocatedIPs, err := getUsedIPsFromAddressRanges(ipamAllocation.Addresses)
					if err != nil {
						return nil, err
					}
					// check if the current allocation is compatible with the IPAMPool being applied
					err = checkRangeAllocation(currentAllocatedIPs, string(dcIPAMPoolCfg.PoolCIDR), int(dcIPAMPoolCfg.AllocationRange))
					if err != nil {
						return nil, err
					}
					for _, ip := range currentAllocatedIPs {
						dcIPAMPoolUsageMap.setUsed(ipamAllocation.Datacenter, ip)
					}
				case "prefix":
					// check if the current allocation is compatible with the IPAMPool being applied
					err := checkPrefixAllocation(string(ipamAllocation.CIDR), string(dcIPAMPoolCfg.PoolCIDR), int(dcIPAMPoolCfg.AllocationPrefix))
					if err != nil {
						return nil, err
					}
					dcIPAMPoolUsageMap.setUsed(ipamAllocation.Datacenter, string(ipamAllocation.CIDR))
				}
			}
		}
	}

	return dcIPAMPoolUsageMap, nil
}

func (p ipam) generateNewAllocationsForPool(ipamPool IPAMPool, dcIPAMPoolUsageMap datacenterIPAMPoolUsageMap) ([]IPAMAllocation, error) {
	newClustersAllocations := []IPAMAllocation{}

	for dc, dcClusters := range p.datacenterAllocations {
		for _, cluster := range dcClusters {
			dcIPAMPoolCfg, isDCConfigured := ipamPool.Datacenters[dc]
			if !isDCConfigured {
				// Cluster datacenter is not configured in the IPAM pool spec, so nothing to do for it
				continue
			}

			isClusterAlreadyAllocatedForPool := false
			for _, clusterAllocation := range cluster.IPAMAllocations {
				if clusterAllocation.IPAMPoolName == ipamPool.Name {
					isClusterAlreadyAllocatedForPool = true
					break
				}
			}
			if isClusterAlreadyAllocatedForPool {
				// skip because pool is already allocated for cluster
				continue
			}

			newClustersAllocation := IPAMAllocation{
				IPAMPoolName: ipamPool.Name,
				Cluster:      cluster.Name,
				Datacenter:   dc,
				Type:         dcIPAMPoolCfg.Type,
			}

			switch dcIPAMPoolCfg.Type {
			case "range":
				addresses, err := findFirstFreeRangesOfPool(dc, string(dcIPAMPoolCfg.PoolCIDR), int(dcIPAMPoolCfg.AllocationRange), dcIPAMPoolUsageMap)
				if err != nil {
					return nil, err
				}
				newClustersAllocation.Addresses = addresses
			case "prefix":
				subnetCIDR, err := findFirstFreeSubnetOfPool(dc, string(dcIPAMPoolCfg.PoolCIDR), int(dcIPAMPoolCfg.AllocationPrefix), dcIPAMPoolUsageMap)
				if err != nil {
					return nil, err
				}
				newClustersAllocation.CIDR = subnetCIDR
			}

			newClustersAllocations = append(newClustersAllocations, newClustersAllocation)
		}
	}

	return newClustersAllocations, nil
}
