package ipam

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPAMPoolReconcile(t *testing.T) {
	testCases := []struct {
		name                               string
		initialDatacenterAllocations       map[string][]IPAMAllocation
		ipamPoolSpecs                      []IPAMPoolSpec
		expectedFinalDatacenterAllocations map[string][]IPAMAllocation
		expectedError                      error
	}{
		{
			name: "range: base case",
			initialDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.3-192.168.1.4",
						},
					},
					{
						Type:      "range",
						Addresses: []string{},
					},
				},
				"azure-as-2": {
					{
						Type:      "range",
						Addresses: []string{},
					},
				},
			},
			ipamPoolSpecs: []IPAMPoolSpec{
				{
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
			},
			expectedFinalDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.3-192.168.1.4",
						},
					},
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.0-192.168.1.2",
							"192.168.1.5-192.168.1.9",
						},
					},
				},
				"azure-as-2": {
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.0-192.168.1.15",
						},
					},
				},
			},
		},
		{
			name: "range: applying same spec twice",
			initialDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.3-192.168.1.4",
						},
					},
					{
						Type:      "range",
						Addresses: []string{},
					},
				},
				"azure-as-2": {
					{
						Type:      "range",
						Addresses: []string{},
					},
				},
			},
			ipamPoolSpecs: []IPAMPoolSpec{
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"azure-as-2": {
							Type:            "range",
							PoolCIDR:        "192.168.1.0/27",
							AllocationRange: 16,
						},
						"aws-eu-1": {
							Type:            "range",
							PoolCIDR:        "192.168.1.0/27",
							AllocationRange: 8,
						},
					},
				},
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"azure-as-2": {
							Type:            "range",
							PoolCIDR:        "192.168.1.0/27",
							AllocationRange: 16,
						},
						"aws-eu-1": {
							Type:            "range",
							PoolCIDR:        "192.168.1.0/27",
							AllocationRange: 8,
						},
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]IPAMAllocation{
				"azure-as-2": {
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.0-192.168.1.15",
						},
					},
				},
				"aws-eu-1": {
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.3-192.168.1.4",
						},
					},
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.0-192.168.1.2",
							"192.168.1.5-192.168.1.9",
						},
					},
				},
			},
		},
		{
			name: "range: changing spec shouldn't change allocations",
			initialDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.3-192.168.1.4",
						},
					},
					{
						Type:      "range",
						Addresses: []string{},
					},
				},
				"azure-as-2": {
					{
						Type:      "range",
						Addresses: []string{},
					},
				},
			},
			ipamPoolSpecs: []IPAMPoolSpec{
				{
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
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"aws-eu-1": {
							Type:            "range",
							PoolCIDR:        "192.168.1.0/26",
							AllocationRange: 16,
						},
						"azure-as-2": {
							Type:            "range",
							PoolCIDR:        "192.168.1.0/26",
							AllocationRange: 16,
						},
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.3-192.168.1.4",
						},
					},
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.0-192.168.1.2",
							"192.168.1.5-192.168.1.9",
						},
					},
				},
				"azure-as-2": {
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.0-192.168.1.15",
						},
					},
				},
			},
		},
		{
			name: "prefix: base case",
			initialDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "prefix",
					},
				},
			},
			ipamPoolSpecs: []IPAMPoolSpec{
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"aws-eu-1": {
							Type:             "prefix",
							PoolCIDR:         "192.168.1.0/16",
							AllocationPrefix: 28,
						},
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "prefix",
						CIDR: "192.168.0.0/28",
					},
				},
			},
		},
		{
			name: "prefix: already allocated cluster",
			initialDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "prefix",
						CIDR: "192.168.0.0/28",
					},
				},
			},
			ipamPoolSpecs: []IPAMPoolSpec{
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"aws-eu-1": {
							Type:             "prefix",
							PoolCIDR:         "192.168.1.0/16",
							AllocationPrefix: 28,
						},
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "prefix",
						CIDR: "192.168.0.0/28",
					},
				},
			},
		},
		{
			name: "prefix: allocation for new cluster in different datacenters",
			initialDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "prefix",
						CIDR: "192.168.0.0/28",
					},
					{
						Type: "prefix",
					},
				},
				"azure-as-2": {
					{
						Type: "prefix",
					},
				},
			},
			ipamPoolSpecs: []IPAMPoolSpec{
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"aws-eu-1": {
							Type:             "prefix",
							PoolCIDR:         "192.168.1.0/16",
							AllocationPrefix: 28,
						},
						"azure-as-2": {
							Type:             "prefix",
							PoolCIDR:         "192.168.1.0/16",
							AllocationPrefix: 28,
						},
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "prefix",
						CIDR: "192.168.0.0/28",
					},
					{
						Type: "prefix",
						CIDR: "192.168.0.16/28",
					},
				},
				"azure-as-2": {
					{
						Type: "prefix",
						CIDR: "192.168.0.0/28",
					},
				},
			},
		},
		{
			name: "prefix: changing spec shouldn't change allocations",
			initialDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "prefix",
						CIDR: "192.168.0.0/28",
					},
					{
						Type: "prefix",
					},
				},
			},
			ipamPoolSpecs: []IPAMPoolSpec{
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"aws-eu-1": {
							Type:             "prefix",
							PoolCIDR:         "192.169.1.0/16",
							AllocationPrefix: 27,
						},
					},
				},
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"aws-eu-1": {
							Type:             "prefix",
							PoolCIDR:         "192.170.2.0/16",
							AllocationPrefix: 20,
						},
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "prefix",
						CIDR: "192.168.0.0/28",
					},
					{
						Type: "prefix",
						CIDR: "192.169.0.0/27",
					},
				},
			},
		},
		{
			name: "from range to prefix and from prefix to range",
			initialDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{},
				},
				"azure-as-2": {
					{},
				},
			},
			ipamPoolSpecs: []IPAMPoolSpec{
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"aws-eu-1": {
							Type:            "range",
							PoolCIDR:        "192.168.1.0/28",
							AllocationRange: 2,
						},
					},
				},
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"aws-eu-1": {
							Type:             "prefix",
							PoolCIDR:         "192.169.2.0/16",
							AllocationPrefix: 20,
						},
						"azure-as-2": {
							Type:             "prefix",
							PoolCIDR:         "192.169.2.0/16",
							AllocationPrefix: 20,
						},
					},
				},
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"aws-eu-1": {
							Type:            "range",
							PoolCIDR:        "192.168.1.0/28",
							AllocationRange: 2,
						},
						"azure-as-2": {
							Type:            "range",
							PoolCIDR:        "192.169.2.0/20",
							AllocationRange: 4096,
						},
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]IPAMAllocation{
				"aws-eu-1": {
					{
						Type: "range",
						Addresses: []string{
							"192.168.1.0-192.168.1.1",
						},
					},
				},
				"azure-as-2": {
					{
						Type: "prefix",
						CIDR: "192.169.0.0/20",
					},
				},
			},
		},
		{
			name:                         "no cluster deployed in any datacenter",
			initialDatacenterAllocations: map[string][]IPAMAllocation{},
			ipamPoolSpecs: []IPAMPoolSpec{
				{
					Datacenters: map[string]IPAMPoolDatacenterSettings{
						"aws-eu-1": {
							Type:            "prefix",
							PoolCIDR:        "192.168.1.0/28",
							AllocationRange: 8,
						},
					},
				},
			},
			expectedFinalDatacenterAllocations: map[string][]IPAMAllocation{},
			expectedError:                      fmt.Errorf("unknown datacenter"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ipam := newIPAM(tc.initialDatacenterAllocations)
			for _, spec := range tc.ipamPoolSpecs {
				err := ipam.reconcile(spec)
				assert.Equal(t, tc.expectedError, err)
			}
			assert.Equal(t, tc.expectedFinalDatacenterAllocations, ipam.datacenterAllocations)
		})
	}
}
