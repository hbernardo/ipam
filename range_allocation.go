package ipam

import (
	"fmt"
	"net"
	"strings"
)

func getUsedIPsFromAddressRanges(addressRanges []string) ([]string, error) {
	usedIPs := []string{}

	for _, addressRange := range addressRanges {
		ipRange := strings.SplitN(addressRange, "-", 2)
		if len(ipRange) != 2 {
			return nil, fmt.Errorf("wrong ip range format")
		}
		firstIP := net.ParseIP(ipRange[0])
		if firstIP == nil {
			return nil, fmt.Errorf("wrong ip format")
		}
		lastIP := net.ParseIP(ipRange[1])
		if lastIP == nil {
			return nil, fmt.Errorf("wrong ip format")
		}
		for ip := firstIP; !ip.Equal(lastIP); ip = incIP(ip) {
			usedIPs = append(usedIPs, ip.String())
		}
		usedIPs = append(usedIPs, lastIP.String())
	}

	return usedIPs, nil
}

func checkRangeAllocation(ips []string, poolCIDR string, allocationRange int) error {
	if allocationRange != len(ips) {
		return errIncompatiblePool
	}

	_, poolSubnet, err := net.ParseCIDR(poolCIDR)
	if err != nil {
		return err
	}

	for _, ip := range ips {
		if !poolSubnet.Contains(net.ParseIP(ip)) {
			return errIncompatiblePool
		}
	}

	return nil
}

func calculateRangeFreeIPsFromDatacenterPool(dc, poolCIDR string, dcIPAMPoolUsageMap datacenterIPAMPoolUsageMap) ([]string, error) {
	rangeFreeIPs := []string{}

	ip, ipNet, err := net.ParseCIDR(poolCIDR)
	if err != nil {
		return nil, err
	}
	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); ip = incIP(ip) {
		if dcIPAMPoolUsageMap.isUsed(dc, ip.String()) {
			continue
		}
		rangeFreeIPs = append(rangeFreeIPs, ip.String())
	}

	return rangeFreeIPs, nil
}

func findFirstFreeRangesOfPool(dc, poolCIDR string, allocationRange int, dcIPAMPoolUsageMap datacenterIPAMPoolUsageMap) ([]string, error) {
	addressRanges := []string{}

	rangeFreeIPs, err := calculateRangeFreeIPsFromDatacenterPool(dc, poolCIDR, dcIPAMPoolUsageMap)
	if err != nil {
		return nil, err
	}

	if allocationRange > len(rangeFreeIPs) {
		return nil, fmt.Errorf("there is no enough free IPs available for pool")
	}

	rangeFreeIPsIterator := 0
	firstAddressRangeIP := rangeFreeIPs[rangeFreeIPsIterator]
	for j := 0; j < allocationRange; j++ {
		ipToAllocate := rangeFreeIPs[rangeFreeIPsIterator]
		dcIPAMPoolUsageMap.setUsed(dc, ipToAllocate)
		rangeFreeIPsIterator++
		// if no next ip to allocate or next ip is not the next one, close a new address range
		if j+1 == allocationRange || !isTheNextIP(rangeFreeIPs[rangeFreeIPsIterator], ipToAllocate) {
			addressRange := fmt.Sprintf("%s-%s", firstAddressRangeIP, ipToAllocate)
			addressRanges = append(addressRanges, addressRange)
			if j+1 < allocationRange {
				firstAddressRangeIP = rangeFreeIPs[rangeFreeIPsIterator]
			}
		}
	}

	return addressRanges, nil
}
