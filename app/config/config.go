package config

import (
	"log"

	"github.com/BurntSushi/toml"
	"github.com/smetroid/d3d-api/app/auth/ldap"
	"github.com/smetroid/d3d-api/app/auth/oauth"
	"github.com/smetroid/d3d-api/app/db/postgres"
	// "github.com/allen13/golerta/app/auth/oauth"
)

type SamusConfig struct {
	Samus    samus
	Ldap     ldap.LDAPAuthProvider
	OAuth    oauth.OAuthAuthProvider
	Postgres postgres.Postgres
}

type samus struct {
	BindAddr       string `toml:"bind_addr"`
	SigningKey     string `toml:"signing_key"`
	AuthProvider   string `toml:"auth_provider"`
	LogDAGRequests bool   `toml:"log_dag_requests"`
	LogEdgeRequests bool  `toml:"log_edge_requests"`
	LogNodeRequests bool  `toml:"log_node_requests"`
	LogMenuRequests bool  `toml:"log_menu_requests"`
	TLSEnabled     bool   `toml:"tls_enabled"`
	TLSCert        string `toml:"tls_cert"`
	TLSKey         string `toml:"tls_key"`
	TLSAutoEnabled bool   `toml:"tls_auto_enabled"`
	TLSAutoHosts   string `toml:"tls_auto_hosts"`
}

func BuildConfig(configFile string) (config SamusConfig) {
	_, err := toml.DecodeFile(configFile, &config)

	if err != nil {
		log.Fatal("config file error: " + err.Error())
	}

	setDefaultConfigs(&config)
	return
}

func setDefaultConfigs(config *SamusConfig) {
	if config.Samus.AuthProvider == "" {
		config.Samus.AuthProvider = "ldap"
	}
}
