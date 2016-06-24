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

	"flag"

	"github.com/containernetworking/cni/pkg/ip"
	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/ns"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	. "github.com/projectcalico/calico-cni/utils"
	"github.com/projectcalico/libcalico/lib"
)

var hostname string

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()

	hostname, _ = os.Hostname()
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
	if err := AddIgnoreUnknownArgs(); err != nil {
		return err
	}

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

	runningUnderK8s, err := RunningUnderK8s(args)
	if err != nil {
		return err
	}

	//var orchestratorID, workloadID string
	var result *types.Result
	if runningUnderK8s {
		result, err = CmdAddK8s(args, conf)
		if err != nil {
			return err
		}
	} else {
		// Not running in an environment that needs special casing.
		workloadID := args.ContainerID
		orchestratorID := "cni"
		profileID := conf.Name

		theEndpoint, err := libcalico.GetEndpoint(
			etcd, libcalico.Workload{
				Hostname:       hostname,
				OrchestratorID: orchestratorID,
				WorkloadID:     workloadID})

		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "Calico CNI checking for existing endpoint. endpoint=%v\n", theEndpoint)

		if theEndpoint != nil {
			// Don't create the veth or do any networking. Just update the profile on the endpoint. The profile will
			// be created if needed during the profile processing step.

			// There's an existing endpoint - no need to create another. Find the IP address from the endpoint
			// and use that in the response.
			fmt.Fprintf(os.Stderr, "Calico CNI appending networking profile=%s\n", profileID)
			theEndpoint.ProfileIDs = append(theEndpoint.ProfileIDs, profileID)
			result, err = createResultFromIP(theEndpoint.IPv4Nets[0])
			if err != nil {
				return err
			}
		} else {
			// There's no existing endpoint, so we need to do the following:
			// 1) Call the configured IPAM plugin to get IP address(es)
			// 2) Create the veth, configuring it on both the host and container namespace.
			// 3) Configure the calico endpoint

			// 1) run the IPAM plugin and make sure there's an IP address returned.
			result, err = ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
			if err != nil {
				return err
			}

			// TODO - the code doesn't actually handle IPv6 properly yet...
			if result.IP4 == nil && result.IP6 == nil {
				return errors.New("IPAM plugin did not return any IP addresses")
			}

			// 2) Set up the veth
			hostVethName, err := DoNetworking(args, conf, result)
			if err != nil {
				return err
			}

			// 3) Create the endpoint object
			theEndpoint = &libcalico.Endpoint{}
			theEndpoint.Name = hostVethName
			theEndpoint.Labels = map[string]string{} //TODO is this needed?
			theEndpoint.ProfileIDs = []string{profileID}
			theEndpoint.OrchestratorID = orchestratorID
			theEndpoint.WorkloadID = workloadID
			theEndpoint.Hostname = hostname
			theEndpoint.Mac = "EE:EE:EE:EE:EE:EE"
			theEndpoint.State = "active"

			if result.IP4 != nil {
				theEndpoint.IPv4Nets = []string{result.IP4.IP.String()}
			} else {
				theEndpoint.IPv4Nets = []string{}
			}

			if result.IP6 != nil {
				theEndpoint.IPv6Nets = []string{result.IP6.IP.String()}
			} else {
				theEndpoint.IPv6Nets = []string{}
			}
			fmt.Fprintf(os.Stderr, "Calico CNI using IPv4=%s IPv6=%s\n", theEndpoint.IPv4Nets, theEndpoint.IPv6Nets)
		}

		// Write the endpoint object (either the newly created one, or the updated one with a new ProfileIDs).
		if err := theEndpoint.Write(etcd); err != nil {
			return err
		}
	}

	// Handle profile creation.
	// If Kubernetes is being used then profiles only need to be created if there is no policy block in the network
	// config. If there is a policy block then "proper" policy is being used and the policy controller handles
	// profile creation.
	if !runningUnderK8s || conf.Policy == nil {
		//TODO - this is the wrong test. It should be on the policy type
		// Start by checking if the profile already exists. If it already exists then there is no work to do (the CNI plugin never updates a profile).
		exists, err := libcalico.ProfileExists(conf.Name, etcd)
		if err != nil {
			return err
		}

		if !exists {
			// The profile doesn't exist so needs to be created. The rules vary depending on whether k8s is being used.
			// Under k8s (without full policy support) the rule is permissive and allows all traffic.
			// Otherwise, incoming traffic is only allowed from profiles with the same tag.
			k8sInboundRule := []libcalico.Rule{libcalico.Rule{Action: "allow"}}
			tagInboundRule := []libcalico.Rule{libcalico.Rule{Action: "allow", SrcTag: conf.Name}}
			fmt.Fprintf(os.Stderr, "Calico CNI creating profile. profile=%s\n", conf.Name)

			var inboundRule []libcalico.Rule
			if runningUnderK8s {
				inboundRule = k8sInboundRule
			} else {
				inboundRule = tagInboundRule
			}

			profile := libcalico.Profile{
				ID: conf.Name,
				Rules: libcalico.Rules{
					Inbound:  inboundRule,
					Outbound: []libcalico.Rule{libcalico.Rule{Action: "allow"}}},
				Tags: []string{conf.Name}}
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

	if err := AddIgnoreUnknownArgs(); err != nil {
		return err
	}

	// Always try to release the address
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

	runningUnderK8s := string(k8sArgs.K8S_POD_NAMESPACE) != "" && string(k8sArgs.K8S_POD_NAME) != ""
	if runningUnderK8s {
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
		Hostname:       hostname,
		OrchestratorID: orchestratorId,
		WorkloadID:     workloadID}
	if err := workload.Delete(etcd); err != nil {
		return err
	}

	// Only try to delete the device if a namespace was passed in
	if args.Netns != "" {
		err := ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
			var err error
			_, err = ip.DelLinkByNameAddr(args.IfName, netlink.FAMILY_V4)
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
	// Display the version on "-v", otherwise just delagate to the skel code.
	// Use a new flag set so as not to conflict with existing libaries which use "flag"
	flagSet := flag.NewFlagSet("Calico", flag.ExitOnError)

	version := flagSet.Bool("v", false, "Display version")
	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if *version {
		fmt.Println(VERSION)
		os.Exit(0)
	}
	skel.PluginMain(cmdAdd, cmdDel)
}
