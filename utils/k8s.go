// +build k8s

package utils

//
//import (
//	"fmt"
//	"os"
//
//	k8sRestClient "k8s.io/kubernetes/pkg/client/restclient"
//	k8sClient "k8s.io/kubernetes/pkg/client/unversioned"
//)
//
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
