package ipam

import (
	"fmt"
	"net"
)

func checkPrefixAllocation(subnetCIDR, poolCIDR string, allocationPrefix int) error {
	subnetIP, subnet, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return err
	}

	subnetPrefix, _ := subnet.Mask.Size()
	if allocationPrefix != subnetPrefix {
		return errIncompatiblePool
	}

	_, poolSubnet, err := net.ParseCIDR(poolCIDR)
	if err != nil {
		return err
	}

	poolPrefix, poolBits := poolSubnet.Mask.Size()
	if subnetPrefix < poolPrefix {
		return errIncompatiblePool
	}
	if subnetPrefix > poolBits {
		return errIncompatiblePool
	}

	if !poolSubnet.Contains(subnetIP) {
		return errIncompatiblePool
	}

	return nil
}

func findFirstFreeSubnetOfPool(dc, poolCIDR string, subnetPrefix int, dcIPAMPoolUsageMap datacenterIPAMPoolUsageMap) (string, error) {
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
		if !dcIPAMPoolUsageMap.isUsed(dc, possibleSubnet.String()) {
			dcIPAMPoolUsageMap.setUsed(dc, possibleSubnet.String())
			return possibleSubnet.String(), nil
		}
	}

	return "", fmt.Errorf("cannot find free subnet")
}
