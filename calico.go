// Copyright 2015 Tigera Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/vishvananda/netlink"

	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/projectcalico/libcalico/lib"
	. "github.com/projectcalico/calico-cni/utils"
	"flag"
	"net"
)

var hostname string

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()

	hostname, _ = os.Hostname()
}

func cmdAddK8s(args *skel.CmdArgs, k8sArgs K8sArgs, conf NetConf, theEndpoint *libcalico.Endpoint) (*types.Result, *libcalico.Endpoint, error) {
	var err error
	var result *types.Result

	profileID := fmt.Sprintf("k8s_ns.%s", k8sArgs.K8S_POD_NAMESPACE)

	if theEndpoint != nil {
		// This happens when Docker or the node restarts. K8s calls CNI with the same details as before.
		// Do the networking but no etcd changes should be required.
		// There's an existing endpoint - no need to create another. Find the IP address from the endpoint
		// and use that in the response.
		result, err = createResultFromIP(theEndpoint.IPv4Nets[0])
		if err != nil {
			return nil, nil, err
		}

		// We're assuming that the endpoint is fine and doesn't need to be changed.
		// However, the veth does need to be recreated since the namespace has been lost.
		_, err := DoNetworking(args, conf, result)
		if err != nil {
			return nil, nil, err
		}
		// TODO - what if labels changed during the restart?

	} else {
		// There's no existing endpoint, so we need to do the following:
		// 1) Call the configured IPAM plugin to get IP address(es)
		// 2) Create the veth, configuring it on both the host and container namespace.
		// 3) Configure the calico endpoint

		// 1) run the IPAM plugin and make sure there's an IPv4 address
		result, err = ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
		if err != nil {
			return nil, nil, err
		}
		if result.IP4 == nil {
			return nil, nil, errors.New("IPAM plugin returned missing IPv4 config")
		}

		// 2) Set up the veth
		hostVethName, err := DoNetworking(args, conf, result)
		if err != nil {
			return nil, nil, err
		}

		// 3) Update the endpoint
		labels, err := GetK8sLabels(conf, k8sArgs)
		if err != nil {
			return nil, nil, err
		}
		theEndpoint = &libcalico.Endpoint{}
		theEndpoint.Name = hostVethName
		theEndpoint.Labels = labels
		theEndpoint.ProfileID = []string{profileID}
	}

	return result, theEndpoint, nil
}

func cmdAddNonK8s(args *skel.CmdArgs, conf NetConf, theEndpoint *libcalico.Endpoint) (*types.Result, *libcalico.Endpoint, error) {
	var err error
	var result *types.Result

	profileID := conf.Name

	if theEndpoint != nil {
		// Don't create the veth or do any networking. Just update the profile on the endpoint
		// (TODO - creating the profile if required)
		// There's an existing endpoint - no need to create another. Find the IP address from the endpoint
		// and use that in the response.
		theEndpoint.ProfileID = append(theEndpoint.ProfileID, profileID)
		result, err = createResultFromIP(theEndpoint.IPv4Nets[0])
		if err != nil {
			return nil, nil, err
		}
	} else {
		// There's no existing endpoint, so we need to do the following:
		// 1) Call the configured IPAM plugin to get IP address(es)
		// 2) Create the veth, configuring it on both the host and container namespace.
		// 3) Configure the calico endpoint

		// 1) run the IPAM plugin and make sure there's an IPv4 address
		result, err = ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
		if err != nil {
			return nil, nil, err
		}
		if result.IP4 == nil {
			return nil, nil, errors.New("IPAM plugin returned missing IPv4 config")
		}

		// 2) Set up the veth
		hostVethName, err := DoNetworking(args, conf, result)
		if err != nil {
			return nil, nil, err
		}

		// 3) Update the endpoint
		theEndpoint = &libcalico.Endpoint{}
		theEndpoint.Name = hostVethName
		theEndpoint.Labels = map[string]string{} //TODO is this needed?
		theEndpoint.ProfileID = []string{profileID}
	}
	return result, theEndpoint, nil
}

func createResultFromIP(ip string) (*types.Result, error) {
	existingIPv4 := types.IPConfig{}
	theIP := fmt.Sprintf(`{"ip": "%s"}`, ip)
	err := existingIPv4.UnmarshalJSON([]byte(theIP))
	if err != nil {
		return nil, err
	}
	return &types.Result{IP4: &existingIPv4}, nil

}

func cmdAdd(args *skel.CmdArgs) error {
	AddIgnoreUnknownArgs()
	var orchestratorID, workloadID string
	var err error

	// Unmarshall the network config, and perform validation
	conf := NetConf{}
	if err := json.Unmarshal(args.StdinData, &conf); err != nil {
		return fmt.Errorf("failed to load netconf: %v", err)
	}

	if err := ValidateNetworkName(conf.Name); err != nil {
		return err
	}

	etcd, err := libcalico.GetKeysAPI(conf.EtcdAuthority, conf.EtcdEndpoints)
	if err != nil {
		return err
	}

	// Determine if running under k8s by checking the CNI args
	k8sArgs := K8sArgs{}
	if args.Args != "" {
		err := LoadArgs(args.Args, &k8sArgs)
		if err != nil {
			return err
		}
	}
	RunningUnderK8s := string(k8sArgs.K8S_POD_NAMESPACE) != "" && string(k8sArgs.K8S_POD_NAME) != ""

	if RunningUnderK8s {
		workloadID = fmt.Sprintf("%s.%s", k8sArgs.K8S_POD_NAMESPACE, k8sArgs.K8S_POD_NAME)
		orchestratorID = "k8s"
	} else {
		workloadID = args.ContainerID
		orchestratorID = "cni"
	}

	// Get an existing workload/endpoint (if one exists).
	theEndpoint, err := libcalico.GetEndpoint(
		etcd, libcalico.Workload{
			Hostname: hostname,
			OrchestratorID: orchestratorID,
			WorkloadID: workloadID})

	fmt.Fprintf(os.Stderr, "Calico CNI checking for existing endpoint. endpoint=%v\n", theEndpoint)

	if err != nil {
		return err
	}

	var result *types.Result
	if RunningUnderK8s {
		result, theEndpoint, err = cmdAddK8s(args, k8sArgs, conf, theEndpoint)
		if err != nil {
			return err
		}
	} else {
		result, theEndpoint, err = cmdAddNonK8s(args, conf, theEndpoint)
				if err != nil {
			return err
		}
	}

	theEndpoint.OrchestratorID = orchestratorID
	theEndpoint.WorkloadID = workloadID
	theEndpoint.Hostname = hostname
	theEndpoint.Mac = "EE:EE:EE:EE:EE:EE"
	theEndpoint.State = "active"
	theEndpoint.IPv6Nets = []string{}
	theEndpoint.IPv4Nets = []string{result.IP4.IP.String()}

	fmt.Fprintf(os.Stderr, "Calico CNI using IPv4=%s\n", result.IP4.IP.String())
	// Write the endpoint object (either the newly created one, or the updated one with a new profileID).
	if err := theEndpoint.Write(etcd); err != nil {
		return err
	}

	// Handle profile creation.
	// If Kubernetes is being used then profiles only need to be created if there is no policy block in the network
	// config. If there is a policy block then "proper" policy is being used and the policy controller handles
	// profile creation.
	if ! RunningUnderK8s || conf.Policy == nil {
		//TODO - this is the wrong test. It should be on the policy type
		// Start by checking if the profile already exists. If it already exists then there is no work to do (the CNI plugin never updates a profile).
		exists, err := libcalico.ProfileExists(conf.Name, etcd)
		if err != nil {
			return err
		}

		if ! exists {
			// The profile doesn't exist so needs to be created. The rules vary depending on whether k8s is being used.
			// Under k8s (without full policy support) the rule is permissive and allows all traffic.
			// Otherwise, incoming traffic is only allowed from profiles with the same tag.
			k8sInboundRule := []libcalico.Rule{libcalico.Rule{Action:"allow"}}
			tagInboundRule := []libcalico.Rule{libcalico.Rule{Action:"allow", SrcTag:conf.Name}}
			fmt.Fprintf(os.Stderr, "Calico CNI creating profile. profile=%s\n", conf.Name)

			var inboundRule []libcalico.Rule
			if RunningUnderK8s {
				inboundRule = k8sInboundRule
			} else {
				inboundRule = tagInboundRule
			}

			profile := libcalico.Profile{
				ID:conf.Name,
				Rules:libcalico.Rules{
					Inbound: inboundRule,
					Outbound:[]libcalico.Rule{libcalico.Rule{Action:"allow"}}},
				Tags:[]string{conf.Name}}
			if err := profile.Write(etcd); err != nil {
				return err
			}
		}
	}

	return result.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	n := NetConf{}
	if err := json.Unmarshal(args.StdinData, &n); err != nil {
		return fmt.Errorf("failed to load netconf: %v", err)
	}

	// Always try to release the address
	AddIgnoreUnknownArgs()
	if err := ipam.ExecDel(n.IPAM.Type, args.StdinData); err != nil {
		return err
	}

	// Always try to clean up the workload/endpoint.
	// First determine if running under k8s to get the right workload and orchestrator IDs
	k8sArgs := K8sArgs{}
	if args.Args != "" {
		err := LoadArgs(args.Args, &k8sArgs)
		if err != nil {
			return err
		}
	}
	var orchestratorId, workloadID string

	RunningUnderK8s := string(k8sArgs.K8S_POD_NAMESPACE) != "" && string(k8sArgs.K8S_POD_NAME) != ""
	if RunningUnderK8s {
		workloadID = fmt.Sprintf("%s.%s", k8sArgs.K8S_POD_NAMESPACE, k8sArgs.K8S_POD_NAME)
		orchestratorId = "k8s"
	} else {
		workloadID = args.ContainerID
		orchestratorId = "cni"
	}

	// Actually remove the workload
	etcd, err := libcalico.GetKeysAPI(n.EtcdAuthority, n.EtcdEndpoints)
	if err != nil {
		return err
	}
	workload := libcalico.Workload{
		Hostname:hostname,
		OrchestratorID:orchestratorId,
		WorkloadID:workloadID}
	if err := workload.Delete(etcd); err != nil {
		return err
	}

	// Only try to delete the device if a namespace was passed in
	if args.Netns != "" {
		var ipn *net.IPNet
		err := ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
			var err error
			ipn, err = ip.DelLinkByNameAddr(args.IfName, netlink.FAMILY_V4)
			return err
		})

		if err != nil {
			return err
		}
	}

	return nil
}

var VERSION string

func main() {
	flagSet := flag.NewFlagSet("Calico", flag.ExitOnError)

	version := flagSet.Bool("v", false, "Display version")
	flagSet.Parse(os.Args[1:])
	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}
	skel.PluginMain(cmdAdd, cmdDel)
}

