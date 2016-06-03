package utils

import (
	"github.com/containernetworking/cni/pkg/ns"
	"net"
	"fmt"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/ip"
	"github.com/vishvananda/netlink"
	"github.com/containernetworking/cni/pkg/skel"
)

var slash32 = net.CIDRMask(32, 32)

func DoNetworking(args *skel.CmdArgs, conf NetConf, result *types.Result) (string, error) {
	hostVethName, _, err := setupContainerVeth(args.Netns, args.IfName, conf.MTU, result)
	if err != nil {
		return "", err
	}

	// Select the first 11 characters of the containerID for the host veth
	newHostVethName := "cali" + args.ContainerID[:Min(11, len(args.ContainerID))]
	if err = setupHostVeth(hostVethName, newHostVethName); err != nil {
		return "", err
	}
	return newHostVethName, nil
}

func setupContainerVeth(netns, ifName string, mtu int, res *types.Result) (string, string, error) {
	var hostVethName, contVethMAC string
	err := ns.WithNetNSPath(netns, func(hostNS ns.NetNS) error {
		hostVeth, contVeth, err := ip.SetupVeth(ifName, mtu, hostNS)

		if err != nil {
			return err
		}

		gw := net.IPv4(169, 254, 1, 1)
		ipn := &net.IPNet{IP: gw, Mask: slash32}
		if err = netlink.RouteAdd(&netlink.Route{
			LinkIndex: contVeth.Attrs().Index,
			Scope:     netlink.SCOPE_LINK,
			Dst:       ipn}); err != nil {
			return fmt.Errorf("failed to add route %v", err)
		}

		_, defNet, _ := net.ParseCIDR("0.0.0.0/0")

		if err = ip.AddRoute(defNet, gw, contVeth); err != nil {
			return fmt.Errorf("failed to add route %v", err)
		}

		address := &netlink.Addr{Label: "", IPNet: &net.IPNet{
			IP:   res.IP4.IP.IP, Mask: slash32} }
		if err = netlink.AddrAdd(contVeth, address); err != nil {
			return fmt.Errorf("failed to add IP addr to %q: %v", ifName, err)
		}

		hostVethName = hostVeth.Attrs().Name

		contVeth, err = netlink.LinkByName(ifName)
		if err != nil {
			err = fmt.Errorf("failed to lookup %q: %v", ifName, err)
			return err
		}

		contVethMAC = contVeth.Attrs().HardwareAddr.String()

		return nil
	})

	return hostVethName, contVethMAC, err
}

func setupHostVeth(vethName, newVethName string) error {
	// hostVeth moved namespaces and may have a new ifindex
	veth, err := netlink.LinkByName(vethName)
	if err != nil {
		return fmt.Errorf("failed to lookup %q: %v", vethName, err)
	}

	if err := netlink.LinkSetDown(veth); err != nil {
		return fmt.Errorf("failed to set %q DOWN: %v", vethName, err)
	}

	if err := netlink.LinkSetName(veth, newVethName); err != nil {
		return fmt.Errorf("failed to rename veth: %v to %v (%v)", vethName, newVethName, err)
	}

	if err := netlink.LinkSetUp(veth); err != nil {
		return fmt.Errorf("failed to set %q UP: %v", vethName, err)
	}

	return nil
}