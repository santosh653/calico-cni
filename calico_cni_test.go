package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/containernetworking/cni/pkg/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	. "github.com/projectcalico/calico-cni/test_utils"
)

// Some ideas for more tests
// Test that both etcd_endpoints and etcd_authity can be used
// Test k8s
// test bad network name
// badly formatted netconf
// vary the MTU
// Existing endpoint

//var _ = Describe("CalicoCniIpam", func() {
//	Describe("Run Calico CNI IPAM plugin", func() {
//})

var _ = Describe("CalicoCni", func() {
	BeforeEach(func() {
		cmd := fmt.Sprintf("etcdctl --endpoints http://%s:2379 rm /calico --recursive | true", os.Getenv("ETCD_IP"))
		session, err := gexec.Start(exec.Command("bash", "-c", cmd), GinkgoWriter, GinkgoWriter)
		Expect(err).ShouldNot(HaveOccurred())
		Eventually(session).Should(gexec.Exit())
	})

	Describe("Run Calico CNI plugin", func() {
		Context("using host-local IPAM", func() {
			netconf := fmt.Sprintf(`
			{
			  "name": "net1",
			  "type": "calico",
			  "etcd_endpoints": "http://%s:2379",
			  "ipam": {
			    "type": "host-local",
			    "subnet": "10.0.0.0/8"
			  }
			}`, os.Getenv("ETCD_IP"))

			It("successfully networks the namespace", func() {
				containerID, session, err := CreateContainer(netconf)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(session).Should(gexec.Exit())
				//fmt.Printf("container_id: %v, result: %s\n", containerID, session.Out.Contents())

				result := types.Result{}
				if err := json.Unmarshal(session.Out.Contents(), &result); err != nil {
					panic(err)
				}
				ip := result.IP4.IP.IP.String()
				Expect(result.IP4.IP.Mask.String()).Should(Equal("ffffffff")) //TODO Should be all FF

				// etcd things:
				// Profile is created with correct details
				Expect(GetEtcdString("/calico/v1/policy/profile/net1/tags")).Should(MatchJSON(`["net1"]`))
				if os.Getenv("PLUGIN") != "calipo" {
					// Python returns a bad extra field
					Expect(GetEtcdString("/calico/v1/policy/profile/net1/rules")).Should(MatchJSON(`{"inbound_rules":[{"action":"allow","src_tag":"net1"}],"outbound_rules":[{"action":"allow"}]}`))
				}

				// The endpoint is created in etcd
				endpoint_path := Cmd(fmt.Sprintf("etcdctl --endpoints http://%s:2379 ls /calico/v1/host/%s/workload/cni/%s --recursive |tail -1", os.Getenv("ETCD_IP"), os.Getenv("HOSTNAME"), containerID))
				endpoint := GetEtcdString(endpoint_path)
				Expect(endpoint_path).Should(ContainSubstring(containerID))
				Expect(endpoint).Should(MatchJSON(fmt.Sprintf(`{"state":"active","name":"cali%s","mac":"EE:EE:EE:EE:EE:EE","profile_ids":["net1"],"ipv4_nets":["%s/8"],"ipv6_nets":[]}`, containerID, ip)))

				// Routes and interface on host - there's is nothing to assert on the routes since felix adds those.
				//fmt.Println(Cmd("ip link show")) // Useful for debugging
				hostVeth := Cmd(fmt.Sprintf("ip addr show cali%s", containerID))
				Expect(hostVeth).Should(ContainSubstring("UP"))
				Expect(hostVeth).Should(ContainSubstring("mtu 1500"))

				// Routes and interface in netns
				containerVeth := Cmd(fmt.Sprintf("ip netns exec %s ip addr show eth0", containerID))
				Expect(containerVeth).Should(ContainSubstring("UP"))
				//Expect(containerVeth).Should(ContainSubstring(ipv4Address))
				Expect(Cmd(fmt.Sprintf("ip netns exec %s ip route", containerID))).Should(Equal(
					"default via 169.254.1.1 dev eth0 \n169.254.1.1 dev eth0  scope link"))

				session, err = DeleteContainer(netconf, containerID)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(session).Should(gexec.Exit())

				session, err = gexec.Start(exec.Command("bash", "-c", EtcdGetCommand(endpoint_path)), GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(session).Should(gexec.Exit(4)) //Exit 4 means the key didn't exist

				session, err = gexec.Start(exec.Command("ip", "netns"), GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(session.Out).Should(gbytes.Say(containerID))
			})
		})

		Context("using calico-ipam IPAM", func() {
			netconf := fmt.Sprintf(`
			{
			  "name": "net1",
			  "type": "calico",
			  "etcd_endpoints": "http://%s:2379",
			  "ipam": {
			    "type": "calico-ipam"
			  }
			}`, os.Getenv("ETCD_IP"))

			It("successfully networks the namespace", func() {
				containerID, session, err := CreateContainer(netconf)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(session).Should(gexec.Exit())
				//fmt.Printf("container_id: %v, result: %s\n", containerID, session.Out.Contents())

				result := types.Result{}
				if err := json.Unmarshal(session.Out.Contents(), &result); err != nil {
					panic(err)
				}
				ip := result.IP4.IP.IP.String()
				Expect(result.IP4.IP.Mask.String()).Should(Equal("ffffffff"))

				// etcd things:
				// Profile is created with correct details
				Expect(GetEtcdString("/calico/v1/policy/profile/net1/tags")).Should(MatchJSON(`["net1"]`))
				if os.Getenv("PLUGIN") != "calipo" {
					// Python returns a bad extra field
					Expect(GetEtcdString("/calico/v1/policy/profile/net1/rules")).Should(MatchJSON(`{"inbound_rules":[{"action":"allow","src_tag":"net1"}],"outbound_rules":[{"action":"allow"}]}`))
				}

				// The endpoint is created in etcd
				endpoint_path := Cmd(fmt.Sprintf("etcdctl --endpoints http://%s:2379 ls /calico/v1/host/%s/workload/cni/%s --recursive |tail -1", os.Getenv("ETCD_IP"), os.Getenv("HOSTNAME"), containerID))
				endpoint := GetEtcdString(endpoint_path)
				Expect(endpoint_path).Should(ContainSubstring(containerID))
				Expect(endpoint).Should(MatchJSON(fmt.Sprintf(`{"state":"active","name":"cali%s","mac":"EE:EE:EE:EE:EE:EE","profile_ids":["net1"],"ipv4_nets":["%s/32"],"ipv6_nets":[]}`, containerID, ip)))

				// Routes and interface on host - there's is nothing to assert on the routes since felix adds those.
				//fmt.Println(Cmd("ip link show")) // Useful for debugging
				hostVeth := Cmd(fmt.Sprintf("ip addr show cali%s", containerID))
				Expect(hostVeth).Should(ContainSubstring("UP"))
				Expect(hostVeth).Should(ContainSubstring("mtu 1500"))

				// Routes and interface in netns
				containerVeth := Cmd(fmt.Sprintf("ip netns exec %s ip addr show eth0", containerID))
				Expect(containerVeth).Should(ContainSubstring("UP"))
				//Expect(containerVeth).Should(ContainSubstring(ipv4Address))
				Expect(Cmd(fmt.Sprintf("ip netns exec %s ip route", containerID))).Should(Equal(
					"default via 169.254.1.1 dev eth0 \n169.254.1.1 dev eth0  scope link"))

				session, err = DeleteContainer(netconf, containerID)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(session).Should(gexec.Exit())

				session, err = gexec.Start(exec.Command("bash", "-c", EtcdGetCommand(endpoint_path)), GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(session).Should(gexec.Exit(4)) //Exit 4 means the key didn't exist

				session, err = gexec.Start(exec.Command("ip", "netns"), GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(session.Out).Should(gbytes.Say(containerID))
			})
		})
	})
})
