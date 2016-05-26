// Copyright 2015 CoreOS, Inc. and Metaswitch Networks
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

	"github.com/appc/cni/pkg/ip"
	"github.com/appc/cni/pkg/ipam"
	"github.com/appc/cni/pkg/ns"
	"github.com/appc/cni/pkg/skel"
	"github.com/appc/cni/pkg/types"
	"net"
	"github.com/projectcalico/libcalico/pkg/endpoint"
	"github.com/projectcalico/libcalico/pkg/profile"
	"crypto/tls"
	"net/http"
	"io/ioutil"
	"crypto/x509"
	"regexp"
	"github.com/projectcalico/libcalico/pkg"
	"github.com/projectcalico/libcalico/pkg/workload"
	"strings"
)

var hostname string

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()

	hostname, _ = os.Hostname()
}

type Policy struct {
	PolicyType              string `json:"type"`
	K8sApiRoot              string `json:"k8s_api_root"`
	K8sAuthToken            string `json:"k8s_auth_token"`
	K8sClientCertificate    string `json:"k8s_client_certificate"`
	K8sClientKey            string `json:"k8s_client_key"`
	K8sCertificateAuthority string `json:"k8s_certificate_authority"`
}

type NetConf struct {
	types.NetConf
	MTU           int  `json:"mtu"`
	EtcdAuthority string `json:"etcd_authority"`
	EtcdEndpoints string `json:"etcd_endpoints"`
	Policy        Policy `json:"policy"`
}

type K8sArgs struct {
	K8S_POD_NAME               string
	K8S_POD_NAMESPACE          string
	K8S_POD_INFRA_CONTAINER_ID string
}

var slash32 = net.CIDRMask(32, 32)

func setupContainerVeth(netns, ifName string, mtu int, res *types.Result) (string, string, error) {
	var hostVethName, contVethMAC string
	err := ns.WithNetNSPath(netns, false, func(hostNS *os.File) error {

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

	netlink.LinkSetName(veth, newVethName)

	if err := netlink.LinkSetUp(veth); err != nil {
		return fmt.Errorf("failed to set %q UP: %v", vethName, err)
	}

	return nil
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getK8sLabels(conf NetConf, k8sargs K8sArgs) (map[string]string, error) {
	// TODO - add in token auth
	var cert tls.Certificate
	tlsConfig := &tls.Config{}
	var err error
	if conf.Policy.K8sClientCertificate != "" && conf.Policy.K8sClientKey != "" {
		// Load client cert and key
		cert, err = tls.LoadX509KeyPair(conf.Policy.K8sClientCertificate,
			conf.Policy.K8sClientKey)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
		tlsConfig.BuildNameToCertificate()
	}

	if conf.Policy.K8sCertificateAuthority != "" {
		// Load CA cert
		caCert, err := ioutil.ReadFile("ssl/ca.pem")
		if err != nil {
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: transport}
	apiRoot := conf.Policy.K8sApiRoot
	if apiRoot == "" {
		apiRoot = "https://10.100.0.1:443/api/v1"
	}
	url := fmt.Sprintf("%s/namespaces/%s/pods/%s", apiRoot,
		k8sargs.K8S_POD_NAMESPACE, k8sargs.K8S_POD_NAME)
	resp, err := client.Get(url)
	//defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var dat map[string]interface{}
	if err := json.Unmarshal(body, &dat); err != nil {
		return nil, err
	}

	metadata := dat["metadata"].(map[string]interface{})
	labels := extractLabels(metadata["labels"])
	labels["calico/k8s_ns"] = k8sargs.K8S_POD_NAMESPACE
	return labels, nil
}

func extractLabels(raw_labels interface{}) map[string]string {
	labels := make(map[string]string)
	if raw_labels != nil {
		for key, value := range raw_labels.(map[string]interface{}) {
			labels[key] = value.(string)
		}
	}

	return labels
}

func validateNetworkName(name string) error {
	matched, err := regexp.MatchString(`^[a-zA-Z0-9_\.\-]+$`, name)
	if err != nil {
		return err
	}
	if ! matched {
		return errors.New("Invalid characters detected in the given network name. " +
		"Only letters a-z, numbers 0-9, and symbols _.- are supported.")
	}
	return nil
}

func cmdAdd(args *skel.CmdArgs) error {
	var orchestratorId, workloadID, profileID string
	var labels map[string]string
	var err error

	conf := NetConf{}
	if err := json.Unmarshal(args.StdinData, &conf); err != nil {
		return fmt.Errorf("failed to load netconf: %v", err)
	}

	if err := validateNetworkName(conf.Name); err != nil {
		return err
	}

	etcd, err := pkg.GetKeysAPI(conf.EtcdAuthority, conf.EtcdEndpoints)
	if err != nil {
		return err
	}

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
		orchestratorId = "k8s"
		labels, err = getK8sLabels(conf, k8sArgs)
		if err != nil {
			return err
		}
		profileID = fmt.Sprintf("k8s_ns.%s", k8sArgs.K8S_POD_NAMESPACE)
	} else {
		workloadID = args.ContainerID
		orchestratorId = "cni"
		labels = map[string]string{}
		profileID = conf.Name

		// Create the profile if needed - name = network_name
		exists, err := profile.ProfileExists(conf.Name, etcd)
		if err != nil {
			return err
		}

		if ! exists {
			profile := profile.Profile{
				ID:conf.Name,
				Rules:profile.Rules{
					Inbound:[]profile.Rule{profile.Rule{Action:"allow", SrcTag:conf.Name}},
					Outbound:[]profile.Rule{profile.Rule{Action:"allow"}}},
				Tags:[]string{conf.Name}}
			if err := profile.Write(etcd); err != nil {
				return err
			}
		}
	}

	// Get an existing workload (if one exists). If it's there then the
	// behavior varies on whether we're running under k8s or not.
	// Under k8s - TODO
	// Otherwise - Just add a new profile to the endpoint.
	found, theendpoint, err := endpoint.GetEndpoint(etcd, workload.Workload{Hostname:hostname, OrchestratorID:orchestratorId, WorkloadID:workloadID})
	var result *types.Result
	if err != nil {
		return err
	}

	if found {
		// There's an existing endpoint
		theendpoint.ProfileID = append(theendpoint.ProfileID, profileID)
		existingIPv4 := types.IPConfig{}
		theIP := fmt.Sprintf(`{"ip": "%s"}`, theendpoint.IPv4Nets[0])
		err = existingIPv4.UnmarshalJSON([]byte(theIP))
		if err != nil {
			return err
		}
		result = &types.Result{IP4: &existingIPv4}
	} else {
		// run the IPAM plugin and make sure there's an IPv4 address
		result, err = ipam.ExecAdd(conf.IPAM.Type, args.StdinData)
		if err != nil {
			return err
		}
		if result.IP4 == nil {
			return errors.New("IPAM plugin returned missing IPv4 config")
		}

		hostVethName, contVethMAC, err := setupContainerVeth(args.Netns, args.IfName, conf.MTU, result)
		if err != nil {
			return err
		}

		// Select the first 11 characters of the containerID for the host veth
		newHostVethName := "cali" + args.ContainerID[:min(11, len(args.ContainerID))]
		if err = setupHostVeth(hostVethName, newHostVethName); err != nil {
			return err
		}

		// Create the endpoint
		theendpoint = endpoint.Endpoint{
			Hostname:hostname,
			OrchestratorID:orchestratorId,
			WorkloadID:workloadID,
			Mac: contVethMAC,
			State:"active",
			Name:newHostVethName,
			IPv4Nets:[]string{result.IP4.IP.String()},
			ProfileID:[]string{profileID},
			IPv6Nets:[]string{},
			Labels:labels}
	}
	if err := theendpoint.Write(etcd); err != nil {
		return err
	}

	return result.Print()
}

func cmdDel(args *skel.CmdArgs) error {
	conf := NetConf{}
	if err := json.Unmarshal(args.StdinData, &conf); err != nil {
		return fmt.Errorf("failed to load netconf: %v", err)
	}

	err := ns.WithNetNSPath(args.Netns, false, func(hostNS *os.File) error {
		var err error
		_, err = ip.DelLinkByNameAddr(args.IfName, netlink.FAMILY_V4)
		return err
	})
	if err != nil {
		return err
	}

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

	// Remove the workload
	etcd, err := pkg.GetKeysAPI(conf.EtcdAuthority, conf.EtcdEndpoints)
	if err != nil {
		return err
	}
	workload := workload.Workload{
		Hostname:hostname,
		OrchestratorID:orchestratorId,
		WorkloadID:workloadID}
	if err := workload.Delete(etcd); err != nil {
		return err
	}

	return ipam.ExecDel(conf.IPAM.Type, args.StdinData)
}

func LoadArgs(args string, container *K8sArgs) error {
	if args == "" {
		return nil
	}

	pairs := strings.Split(args, ";")
	for _, pair := range pairs {
		kv := strings.Split(pair, "=")
		if len(kv) != 2 {
			return fmt.Errorf("ARGS: invalid pair %q", pair)
		}
		keyString := kv[0]
		valueString := kv[1]
		switch {
		case keyString == "K8S_POD_INFRA_CONTAINER_ID":
			container.K8S_POD_INFRA_CONTAINER_ID = valueString
		case keyString == "K8S_POD_NAMESPACE":
			container.K8S_POD_NAMESPACE = valueString
		case keyString == "K8S_POD_NAME":
			container.K8S_POD_NAME = valueString
		}
	}
	return nil
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel)
}

