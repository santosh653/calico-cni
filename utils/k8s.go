// +build k8s

package utils

import (
	"errors"
	"fmt"
	"os"

	"github.com/containernetworking/cni/pkg/ipam"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/projectcalico/libcalico/lib"
)

//
//import (
//	"fmt"
//	"os"
//
//	k8sRestClient "k8s.io/kubernetes/pkg/client/restclient"
//	k8sClient "k8s.io/kubernetes/pkg/client/unversioned"
//)
//

func RunningUnderK8s(args *skel.CmdArgs) (bool, error) {

	// Determine if running under k8s by checking the CNI args
	k8sArgs := K8sArgs{}
	if args.Args != "" {
		err := LoadArgs(args.Args, &k8sArgs)
		if err != nil {
			return nil, err
		}
	}
	return string(k8sArgs.K8S_POD_NAMESPACE) != "" && string(k8sArgs.K8S_POD_NAME) != "", nil
}

func CmdAddK8s(args *skel.CmdArgs, k8sArgs K8sArgs, conf NetConf, (*types.Result, error) {
	var err error
	var result *types.Result

	etcd, err := libcalico.GetKeysAPI(conf.EtcdAuthority, conf.EtcdEndpoints)
	if err != nil {
		return err
	}

	ProfileIDs := fmt.Sprintf("k8s_ns.%s", k8sArgs.K8S_POD_NAMESPACE)

	workloadID = fmt.Sprintf("%s.%s", k8sArgs.K8S_POD_NAMESPACE, k8sArgs.K8S_POD_NAME)
	orchestratorID = "k8s"

	theEndpoint, err := libcalico.GetEndpoint(
		etcd, libcalico.Workload{
			Hostname:       hostname,
			OrchestratorID: orchestratorID,
			WorkloadID:     workloadID})

	fmt.Fprintf(os.Stderr, "Calico CNI checking for existing endpoint. endpoint=%v\n", theEndpoint)

	if err != nil {
		return err
	}

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
		// TODO - the veth name needs updating...

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
		theEndpoint.ProfileIDs = []string{ProfileIDs}
	}

	theEndpoint.OrchestratorID = orchestratorID
	theEndpoint.WorkloadID = workloadID

	return result, theEndpoint, nil
}

//func GetK8sLabels(conf NetConf, k8sargs K8sArgs) (map[string]string, error) {
//	apiRoot := conf.Policy.K8sApiRoot
//	if apiRoot == "" {
//		apiRoot = "https://10.100.0.1:443"
//	}
//
//	fmt.Fprintf(os.Stderr, "Using apiRoot %s", apiRoot)
//
//	//TODO - strip the path parts off - since that what the old format was
//
//	config := k8sRestClient.Config{
//		Host: apiRoot,
//	}
//
//	c, err := k8sClient.New(&config)
//	if err != nil {
//		return nil, err
//	}
//
//	podAPI := c.Pods(k8sargs.K8S_POD_NAMESPACE)
//	pod, err := podAPI.Get(k8sargs.K8S_POD_NAME)
//	if err != nil {
//		return nil, err
//	}
//	labels := pod.Labels
//	labels["calico/k8s_ns"] = k8sargs.K8S_POD_NAMESPACE
//	return labels, nil
//
//	//s, err := c.Services(api.NamespaceDefault).Get("some-service-name")
//	//if err != nil {
//	//	log.Fatalln("Can't get service:", err)
//	//}
//	//fmt.Println("Name:", s.Name)
//	//for p, _ := range s.Spec.Ports {
//	//	fmt.Println("Port:", s.Spec.Ports[p].Port)
//	//	fmt.Println("NodePort:", s.Spec.Ports[p].NodePort)
//	//}
//
//	// TODO - add in token auth
//	//var cert tls.Certificate
//	//tlsConfig := &tls.Config{}
//	//var err error
//	//if conf.Policy.K8sClientCertificate != "" && conf.Policy.K8sClientKey != "" {
//	//	// Load client cert and key
//	//	cert, err = tls.LoadX509KeyPair(conf.Policy.K8sClientCertificate,
//	//		conf.Policy.K8sClientKey)
//	//	if err != nil {
//	//		return nil, err
//	//	}
//	//	tlsConfig.Certificates = []tls.Certificate{cert}
//	//	tlsConfig.BuildNameToCertificate()
//	//}
//	//
//	//if conf.Policy.K8sCertificateAuthority != "" {
//	//	// Load CA cert
//	//	caCert, err := ioutil.ReadFile("ssl/ca.pem")
//	//	if err != nil {
//	//		return nil, err
//	//	}
//	//	caCertPool := x509.NewCertPool()
//	//	caCertPool.AppendCertsFromPEM(caCert)
//	//	tlsConfig.RootCAs = caCertPool
//	//}
//	//
//	//transport := &http.Transport{TLSClientConfig: tlsConfig}
//	//client := &http.Client{Transport: transport}
//	//apiRoot := conf.Policy.K8sApiRoot
//	//if apiRoot == "" {
//	//	apiRoot = "https://10.100.0.1:443/api/v1"
//	//}
//	//url := fmt.Sprintf("%s/namespaces/%s/pods/%s", apiRoot,
//	//	k8sargs.K8S_POD_NAMESPACE, k8sargs.K8S_POD_NAME)
//	//resp, err := client.Get(url)
//	////defer resp.Body.Close()
//	//body, err := ioutil.ReadAll(resp.Body)
//	//if err != nil {
//	//	return nil, err
//	//}
//	//
//	//var dat map[string]interface{}
//	//if err := json.Unmarshal(body, &dat); err != nil {
//	//	return nil, err
//	//}
//	//
//	//metadata := dat["metadata"].(map[string]interface{})
//	//labels := extractLabels(metadata["labels"])
//	//labels["calico/k8s_ns"] = k8sargs.K8S_POD_NAMESPACE
//	//return labels, nil
//}
//
////func extractLabels(raw_labels interface{}) map[string]string {
////	labels := make(map[string]string)
////	if raw_labels != nil {
////		for key, value := range raw_labels.(map[string]interface{}) {
////			labels[key] = value.(string)
////		}
////	}
////
////	return labels
////}
