package main

import (
	"fmt"
	"net/http"
	"crypto/tls"
	"log"
	"io/ioutil"
	"crypto/x509"
)

func main() {
	cert, err := tls.LoadX509KeyPair("ssl/admin.pem", "ssl/admin-key.pem")
	if err != nil {
		log.Fatal(err)
	}

	// Load CA cert
	caCert, err := ioutil.ReadFile("ssl/ca.pem")
	if err != nil {
		log.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Setup HTTPS client
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}
	tlsConfig.BuildNameToCertificate()
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: transport}

	_, err = client.Get("https://172.17.4.101:443/api/v1/namespaces/default/pods")
	if err != nil {
		fmt.Println(err)
	}
}