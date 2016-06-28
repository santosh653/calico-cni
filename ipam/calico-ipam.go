package main

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/projectcalico/libcalico/lib/ipam"
)

func main() {
	skel.PluginMain(cmdAdd, cmdDel)
}

// IPAMConfig represents the IP related network configuration.
type IPAMConfig struct {
	Name          string
	Type          string `json:"type"`
	EtcdAuthority string `json:"etcd_authority"`
	EtcdEndpoints string `json:"etcd_endpoints"`
	AssignIpv4    *bool  `json:"assign_ipv4"` // TODO make sure this defaults to true
	AssignIpv6    *bool  `json:"assign_ipv6"`

	Args *IPAMArgs `json:"-"`
}

type IPAMArgs struct {
	types.CommonArgs
	IP net.IP `json:"ip,omitempty"`
}

type Net struct {
	Name string      `json:"name"`
	IPAM *IPAMConfig `json:"ipam"`
}

func LoadIPAMConfig(bytes []byte, args string) (*IPAMConfig, error) {
	n := Net{}
	if err := json.Unmarshal(bytes, &n); err != nil {
		return nil, err
	}

	if args != "" {
		n.IPAM.Args = &IPAMArgs{}
		err := types.LoadArgs(args, n.IPAM.Args)
		if err != nil {
			return nil, err
		}
	}

	if n.IPAM == nil {
		return nil, fmt.Errorf("missing 'ipam' key")
	}

	// Copy net name into IPAM so not to drag Net struct around
	n.IPAM.Name = n.Name

	return n.IPAM, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	conf, err := LoadIPAMConfig(args.StdinData, args.Args)
	if err != nil {
		return err
	}

	ipamClient, err := ipam.NewIPAMClient()
	if err != nil {
		return err
	}

	// Default to assigning an IPv4 address
	num4 := 1
	if conf.AssignIpv4 != nil && *conf.AssignIpv4 == false {
		num4 = 0
	}

	// Default to NOT assigning an IPv6 address
	num6 := 0
	if conf.AssignIpv6 != nil && *conf.AssignIpv6 == true {
		num6 = 1
	}

	// TODO - Read the IP from CNI_ARGS and use it.
	// TODO - Use the workloadID as the handle (i.e. need to know about k8s)
	// TODO - plumb through hostname
	// TODO - plumb through etcd auth
	// TODO - confirm with Casey if HandleID really needs to be a pointer to a string.
	assignArgs := ipam.AutoAssignArgs{Num4: num4, Num6: num6, HandleID: &args.ContainerID}
	assignedV4, assignedV6, err := ipamClient.AutoAssign(assignArgs)
	if err != nil {
		return err
	}

	r := &types.Result{}

	if conf.AssignIpv4 == nil || (conf.AssignIpv4 != nil && *conf.AssignIpv4 == true) {
		ipV4Network := net.IPNet{IP: assignedV4[0], Mask: net.CIDRMask(32, 32)}
		r.IP4 = &types.IPConfig{IP: ipV4Network}
	}

	if conf.AssignIpv6 != nil && *conf.AssignIpv6 == true {
		ipV6Network := net.IPNet{IP: assignedV6[0], Mask: net.CIDRMask(128, 128)}
		r.IP6 = &types.IPConfig{IP: ipV6Network}
	}

	return r.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	// TODO - Use the workloadID as the handle (i.e. need to know about k8s)
	// Release by handle - which is workloadID.
	ipamClient, err := ipam.NewIPAMClient()
	if err != nil {
		return err
	}
	ipamClient.ReleaseByHandle(args.ContainerID)

	return nil
}
