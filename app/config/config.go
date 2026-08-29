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

	// PostgreSQL is an alias section ([postgresql]) accepted for configs
	// that spell out discrete connection fields; see mergePostgresConfig.
	PostgreSQL postgres.Postgres `toml:"postgresql"`

	Google SocialProvider `toml:"google"`
	GitHub SocialProvider `toml:"github"`
}

// SocialProvider holds one OAuth application's credentials. Empty ClientID
// means the provider is not configured and its routes return 501.
type SocialProvider struct {
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
	RedirectURL  string `toml:"redirect_url"`
}

type samus struct {
	BindAddr        string `toml:"bind_addr"`
	SigningKey      string `toml:"signing_key"`
	AuthProvider    string `toml:"auth_provider"`
	LogDAGRequests  bool   `toml:"log_dag_requests"`
	LogEdgeRequests bool   `toml:"log_edge_requests"`
	LogNodeRequests bool   `toml:"log_node_requests"`
	LogMenuRequests bool   `toml:"log_menu_requests"`
	TLSEnabled      bool   `toml:"tls_enabled"`
	TLSCert         string `toml:"tls_cert"`
	TLSKey          string `toml:"tls_key"`
	TLSAutoEnabled  bool   `toml:"tls_auto_enabled"`
	TLSAutoHosts    string `toml:"tls_auto_hosts"`

	// FrontendOrigin is the single allowed CORS origin and the base for OAuth
	// redirect URLs. CookieSecure is false only for plain-HTTP local dev.
	FrontendOrigin string `toml:"frontend_origin"`
	CookieSecure   bool   `toml:"cookie_secure"`
}

func BuildConfig(configFile string) (config SamusConfig) {
	_, err := toml.DecodeFile(configFile, &config)

	if err != nil {
		log.Fatal("config file error: " + err.Error())
	}

	mergePostgresConfig(&config)
	setDefaultConfigs(&config)
	return
}

// mergePostgresConfig lets a [postgresql] section stand in for [postgres].
// An explicitly configured [postgres] block always wins.
func mergePostgresConfig(config *SamusConfig) {
	if !config.Postgres.Configured() && config.PostgreSQL.Configured() {
		config.Postgres = config.PostgreSQL
	}
	config.PostgreSQL = postgres.Postgres{}
}

func setDefaultConfigs(config *SamusConfig) {
	if config.Samus.AuthProvider == "" {
		config.Samus.AuthProvider = "ldap"
	}
}
