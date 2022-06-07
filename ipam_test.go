package ipam

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPAMPoolReconcile(t *testing.T) {
	testCases := []struct {
		name                               string
		initialDatacenterAllocations       map[string][]Cluster
		ipamPool                           IPAMPool
		expectedFinalDatacenterAllocations map[string][]Cluster
		expectedError                      error
	}{
		{
			name: "range: base case",
			initialDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name:            "c1",
						IPAMAllocations: []IPAMAllocation{},
					},
					{
						Name:            "c2",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
				"azure-as-2": {
					{
						Name:            "c3",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			ipamPool: IPAMPool{
				Name: "pool1",
				Datacenters: map[string]IPAMPoolDatacenterSettings{
					"aws-eu-1": {
						Type:            "range",
						PoolCIDR:        "192.168.1.0/28",
						AllocationRange: 8,
					},
					"azure-as-2": {
						Type:            "range",
						PoolCIDR:        "192.168.1.0/27",
						AllocationRange: 16,
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name: "c1",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "range",
								Addresses: []string{
									"192.168.1.0-192.168.1.7",
								},
							},
						},
					},
					{
						Name: "c2",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "range",
								Addresses: []string{
									"192.168.1.8-192.168.1.15",
								},
							},
						},
					},
				},
				"azure-as-2": {
					{
						Name: "c3",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "range",
								Addresses: []string{
									"192.168.1.0-192.168.1.15",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "range: applying a different pool",
			initialDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name: "c1",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "range",
								Addresses: []string{
									"192.168.1.3-192.168.1.4",
								},
							},
						},
					},
					{
						Name:            "c2",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
				"azure-as-2": {
					{
						Name:            "c3",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			ipamPool: IPAMPool{
				Name: "pool2",
				Datacenters: map[string]IPAMPoolDatacenterSettings{
					"aws-eu-1": {
						Type:            "range",
						PoolCIDR:        "192.168.1.0/27",
						AllocationRange: 8,
					},
					"azure-as-2": {
						Type:            "range",
						PoolCIDR:        "192.168.1.0/27",
						AllocationRange: 16,
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name: "c1",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "range",
								Addresses: []string{
									"192.168.1.3-192.168.1.4",
								},
							},
							{
								IPAMPoolName: "pool2",
								Type:         "range",
								Addresses: []string{
									"192.168.1.0-192.168.1.7",
								},
							},
						},
					},
					{
						Name: "c2",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool2",
								Type:         "range",
								Addresses: []string{
									"192.168.1.8-192.168.1.15",
								},
							},
						},
					},
				},
				"azure-as-2": {
					{
						Name: "c3",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool2",
								Type:         "range",
								Addresses: []string{
									"192.168.1.0-192.168.1.15",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "range: no free ips for allocation",
			initialDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name:            "c1",
						IPAMAllocations: []IPAMAllocation{},
					},
					{
						Name:            "c2",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			ipamPool: IPAMPool{
				Name: "pool1",
				Datacenters: map[string]IPAMPoolDatacenterSettings{
					"aws-eu-1": {
						Type:            "range",
						PoolCIDR:        "192.168.1.0/28",
						AllocationRange: 9,
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name:            "c1",
						IPAMAllocations: []IPAMAllocation{},
					},
					{
						Name:            "c2",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			expectedError: fmt.Errorf("there is no enough free IPs available for pool pool1"),
		},
		{
			name: "cannot apply a pool with a name that was already applied before", // TODO: check if there are cases that we would accept the update
			initialDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name: "c1",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "range",
								Addresses: []string{
									"192.168.1.0-192.168.1.7",
								},
							},
						},
					},
				},
			},
			ipamPool: IPAMPool{
				Name: "pool1",
				Datacenters: map[string]IPAMPoolDatacenterSettings{
					"aws-eu-1": {
						Type:            "range",
						PoolCIDR:        "192.168.1.0/28",
						AllocationRange: 8,
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name: "c1",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "range",
								Addresses: []string{
									"192.168.1.0-192.168.1.7",
								},
							},
						},
					},
				},
			},
			expectedError: fmt.Errorf("pool pool1 is already applied to the cluster c1"),
		},
		{
			name: "prefix: base case",
			initialDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name:            "c1",
						IPAMAllocations: []IPAMAllocation{},
					},
					{
						Name:            "c2",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
				"azure-as-2": {
					{
						Name:            "c3",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			ipamPool: IPAMPool{
				Name: "pool1",
				Datacenters: map[string]IPAMPoolDatacenterSettings{
					"aws-eu-1": {
						Type:             "prefix",
						PoolCIDR:         "192.168.0.0/16",
						AllocationPrefix: 28,
					},
					"azure-as-2": {
						Type:             "prefix",
						PoolCIDR:         "192.168.0.0/16",
						AllocationPrefix: 28,
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name: "c1",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "prefix",
								CIDR:         "192.168.0.0/28",
							},
						},
					},
					{
						Name: "c2",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "prefix",
								CIDR:         "192.168.0.16/28",
							},
						},
					},
				},
				"azure-as-2": {
					{
						Name: "c3",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "prefix",
								CIDR:         "192.168.0.0/28",
							},
						},
					},
				},
			},
		},
		{
			name: "prefix: applying a different pool",
			initialDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name: "c1",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "prefix",
								CIDR:         "192.168.0.0/28",
							},
						},
					},
					{
						Name:            "c2",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
				"azure-as-2": {
					{
						Name:            "c3",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			ipamPool: IPAMPool{
				Name: "pool2",
				Datacenters: map[string]IPAMPoolDatacenterSettings{
					"aws-eu-1": {
						Type:             "prefix",
						PoolCIDR:         "192.168.0.0/20",
						AllocationPrefix: 21,
					},
					"azure-as-2": {
						Type:             "prefix",
						PoolCIDR:         "192.168.0.0/20",
						AllocationPrefix: 21,
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name: "c1",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool1",
								Type:         "prefix",
								CIDR:         "192.168.0.0/28",
							},
							{
								IPAMPoolName: "pool2",
								Type:         "prefix",
								CIDR:         "192.168.0.0/21",
							},
						},
					},
					{
						Name: "c2",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool2",
								Type:         "prefix",
								CIDR:         "192.168.8.0/21",
							},
						},
					},
				},
				"azure-as-2": {
					{
						Name: "c3",
						IPAMAllocations: []IPAMAllocation{
							{
								IPAMPoolName: "pool2",
								Type:         "prefix",
								CIDR:         "192.168.0.0/21",
							},
						},
					},
				},
			},
		},
		{
			name: "prefix: no free subnets for allocation",
			initialDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name:            "c1",
						IPAMAllocations: []IPAMAllocation{},
					},
					{
						Name:            "c2",
						IPAMAllocations: []IPAMAllocation{},
					},
					{
						Name:            "c3",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			ipamPool: IPAMPool{
				Name: "pool1",
				Datacenters: map[string]IPAMPoolDatacenterSettings{
					"aws-eu-1": {
						Type:             "prefix",
						PoolCIDR:         "192.168.0.0/30",
						AllocationPrefix: 31,
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name:            "c1",
						IPAMAllocations: []IPAMAllocation{},
					},
					{
						Name:            "c2",
						IPAMAllocations: []IPAMAllocation{},
					},
					{
						Name:            "c3",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			expectedError: fmt.Errorf("cannot find free subnet"),
		},
		{
			name: "prefix: invalid allocation prefix for pool",
			initialDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name:            "c1",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			ipamPool: IPAMPool{
				Datacenters: map[string]IPAMPoolDatacenterSettings{
					"aws-eu-1": {
						Type:             "prefix",
						PoolCIDR:         "192.168.1.0/28",
						AllocationPrefix: 27,
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name:            "c1",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			expectedError: fmt.Errorf("invalid prefix for subnet"),
		},
		{
			name: "prefix: invalid allocation prefix for pool (2)",
			initialDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name:            "c1",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			ipamPool: IPAMPool{
				Datacenters: map[string]IPAMPoolDatacenterSettings{
					"aws-eu-1": {
						Type:             "prefix",
						PoolCIDR:         "192.168.1.0/28",
						AllocationPrefix: 33,
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]Cluster{
				"aws-eu-1": {
					{
						Name:            "c1",
						IPAMAllocations: []IPAMAllocation{},
					},
				},
			},
			expectedError: fmt.Errorf("invalid prefix for subnet"),
		},
		{
			name:                         "no cluster deployed in any datacenter",
			initialDatacenterAllocations: map[string][]Cluster{},
			ipamPool: IPAMPool{
				Datacenters: map[string]IPAMPoolDatacenterSettings{
					"aws-eu-1": {
						Type:             "prefix",
						PoolCIDR:         "192.168.1.0/28",
						AllocationPrefix: 29,
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]Cluster{},
			expectedError:                      fmt.Errorf("no cluster deployed in datacenter aws-eu-1"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ipam := newIPAM(tc.initialDatacenterAllocations)
			err := ipam.apply(tc.ipamPool)
			assert.Equal(t, tc.expectedError, err)
			assert.Equal(t, tc.expectedFinalDatacenterAllocations, ipam.datacenterAllocations)
		})
	}
}
