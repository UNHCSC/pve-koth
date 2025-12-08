package koth

import (
	"fmt"
	"net"

	"github.com/UNHCSC/pve-koth/config"
	"github.com/UNHCSC/pve-koth/db"
)

func allocateCompetitionSubnet() (*net.IPNet, error) {
	var pool *net.IPNet = config.Config.Network.ParsedPool()
	if pool == nil {
		return nil, fmt.Errorf("network pool not configured")
	}

	var existing, err = db.Competitions.SelectAll()
	if err != nil {
		return nil, fmt.Errorf("fetch competitions: %w", err)
	}

	var used = make(map[string]struct{})
	for _, competition := range existing {
		if competition.NetworkCIDR == "" {
			continue
		}

		if _, netblock, parseErr := net.ParseCIDR(competition.NetworkCIDR); parseErr == nil && netblock != nil {
			used[netblock.String()] = struct{}{}
		}
	}

	var (
		baseIP           = ipToUint32(pool.IP)
		poolPrefix, _    = pool.Mask.Size()
		compPrefix       = config.Config.Network.CompetitionSubnetPrefix
		subnetSize       = uint32(1) << uint32(32-compPrefix)
		availableSubnets = uint32(1) << uint32(compPrefix-poolPrefix)
		allocatedSubnets uint32
	)

	for allocatedSubnets < availableSubnets {
		var subnetStart = baseIP + allocatedSubnets*subnetSize
		var subnet = buildSubnet(subnetStart, compPrefix)
		if subnet == nil {
			break
		}

		if _, taken := used[subnet.String()]; !taken {
			return subnet, nil
		}

		allocatedSubnets++
	}

	return nil, fmt.Errorf("no available /%d subnets remain in pool %s", compPrefix, pool.String())
}

func maxTeamsPerCompetition() int {
	var diff = config.Config.Network.TeamSubnetPrefix - config.Config.Network.CompetitionSubnetPrefix
	if diff <= 0 {
		return 0
	}

	return 1 << diff
}

func teamSubnetBaseIP(compSubnet *net.IPNet, teamIndex int) (uint32, error) {
	if compSubnet == nil {
		return 0, fmt.Errorf("competition subnet is nil")
	}

	if teamIndex < 0 {
		return 0, fmt.Errorf("invalid team index %d", teamIndex)
	}

	var capacity = maxTeamsPerCompetition()
	if capacity == 0 || teamIndex >= capacity {
		return 0, fmt.Errorf("team index %d exceeds available /%d subnets in %s", teamIndex, config.Config.Network.TeamSubnetPrefix, compSubnet.String())
	}

	var base = ipToUint32(compSubnet.IP)
	var teamPrefix = config.Config.Network.TeamSubnetPrefix
	var step = uint32(1) << uint32(32-teamPrefix)

	return base + uint32(teamIndex)*step, nil
}

func hostIPWithinSubnet(subnetBase uint32, subnetPrefix int, hostOffset int) (net.IP, error) {
	var hostCapacity = 1 << (32 - subnetPrefix)
	if hostOffset <= 0 || hostOffset >= hostCapacity-1 {
		return nil, fmt.Errorf("host offset %d is invalid for /%d", hostOffset, subnetPrefix)
	}

	return uint32ToIP(subnetBase + uint32(hostOffset)), nil
}

func buildSubnet(start uint32, prefix int) *net.IPNet {
	var mask = net.CIDRMask(prefix, 32)
	if mask == nil {
		return nil
	}

	var base = start & maskToUint32(mask)
	return &net.IPNet{
		IP:   uint32ToIP(base),
		Mask: mask,
	}
}

func maskToUint32(mask net.IPMask) uint32 {
	var result uint32
	for _, octet := range mask {
		result = (result << 8) | uint32(octet)
	}
	return result
}

func ipToUint32(ip net.IP) uint32 {
	var v = ip.To4()
	if v == nil {
		return 0
	}

	return uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
}

func uint32ToIP(value uint32) net.IP {
	return net.IPv4(
		byte(value>>24),
		byte(value>>16),
		byte(value>>8),
		byte(value),
	).To4()
}
