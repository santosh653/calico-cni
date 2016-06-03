package utils

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

type K8sArgs struct {
	K8S_POD_NAME               string
	K8S_POD_NAMESPACE          string
	K8S_POD_INFRA_CONTAINER_ID string
}

func GetK8sLabels(conf NetConf, k8sargs K8sArgs) (map[string]string, error) {
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