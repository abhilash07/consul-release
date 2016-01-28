package confab_test

import (
	"confab"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	Describe("DefaultConfig", func() {
		It("returns a default configuration", func() {
			config := confab.Config{
				Consul: confab.ConfigConsul{
					RequireSSL: true,
				},
				Path: confab.ConfigPath{
					AgentPath:       "/var/vcap/packages/consul/bin/consul",
					ConsulConfigDir: "/var/vcap/jobs/consul_agent/config",
				},
			}
			Expect(confab.DefaultConfig()).To(Equal(config))
		})
	})

	Describe("ConfigFromJSON", func() {
		It("returns a config given JSON", func() {
			json := []byte(`{
				"node": {
					"name": "nodename",
					"index": 1234
				},
				"path": {
					"agent_path": "/path/to/agent",
					"consul_config_dir": "/consul/config/dir"
				},
				"consul": {
					"agent": {
						"services": {
							"myservice": {
								"name" : "myservicename"	
							}
						},
						"server": true
					},
					"require_ssl": true,
					"encrypt_keys": ["key-1", "key-2"]
				}
			}`)
			config, err := confab.ConfigFromJSON(json)
			Expect(err).NotTo(HaveOccurred())
			Expect(config).To(Equal(confab.Config{
				Path: confab.ConfigPath{
					AgentPath:       "/path/to/agent",
					ConsulConfigDir: "/consul/config/dir",
				},
				Node: confab.ConfigNode{
					Name:  "nodename",
					Index: 1234,
				},
				Consul: confab.ConfigConsul{
					Agent: confab.ConfigAgent{
						Services: map[string]confab.ServiceDefinition{
							"myservice": confab.ServiceDefinition{
								Name: "myservicename",
							},
						},
						Server: true,
					},
					RequireSSL:  true,
					EncryptKeys: []string{"key-1", "key-2"},
				},
			}))
		})

		It("returns a config with default values", func() {
			json := []byte(`{}`)
			config, err := confab.ConfigFromJSON(json)
			Expect(err).NotTo(HaveOccurred())
			Expect(config).To(Equal(confab.Config{
				Path: confab.ConfigPath{
					AgentPath:       "/var/vcap/packages/consul/bin/consul",
					ConsulConfigDir: "/var/vcap/jobs/consul_agent/config",
				},
				Consul: confab.ConfigConsul{
					RequireSSL: true,
				},
			}))
		})

		It("returns an error on invalid json", func() {
			json := []byte(`{%%%{{}{}{{}{}{{}}}}}}}`)
			_, err := confab.ConfigFromJSON(json)
			Expect(err).To(MatchError(ContainSubstring("invalid character")))
		})
	})
})
