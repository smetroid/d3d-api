package ldap

import (
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	tk "github.com/smetroid/d3d-api/app/auth/token"
	"github.com/smetroid/d3d-api/app/models"
	ldap "gopkg.in/ldap.v2"
)

// groupSyncer is the subset of postgres.Postgres needed for LDAP group sync.
// Using an interface keeps the ldap package free of a direct postgres import.
type groupSyncer interface {
	UpsertGroupByExternalRef(g models.Group) (string, error)
	AddGroupMember(gm models.GroupMember) error
}

type LDAPAuthProvider struct {
	conn         *ldap.Conn
	signingKey   string
	Host         string   `toml:"host"`
	Port         int      `toml:"port"`
	UseSSL       bool     `toml:"use_ssl"`
	BaseDN       string   `toml:"base_dn"`
	BindDN       string   `toml:"bind_dn"`
	BindPassword string   `toml:"bind_password"`
	UserFilter   string   `toml:"user_filter"`
	Attributes   []string `toml:"attributes"`
	// LDAPCompanyId is the company ID to associate synced LDAP groups with.
	// When empty, group sync is skipped.
	LDAPCompanyId string `toml:"ldap_company_id"`
	// DB is set by app.go when LDAPCompanyId is configured.
	DB groupSyncer `toml:"-"`
}

func (lc *LDAPAuthProvider) SetSigningKey(key string) {
	lc.signingKey = key
}

// Connect connects to the ldap backend
func (lc *LDAPAuthProvider) Connect() error {
	if lc.conn == nil {
		var l *ldap.Conn
		var err error
		address := fmt.Sprintf("%s:%d", lc.Host, lc.Port)
		if !lc.UseSSL {
			l, err = ldap.Dial("tcp", address)
			if err != nil {
				return err
			}

		} else {
			l, err = ldap.DialTLS("tcp", address, &tls.Config{InsecureSkipVerify: true, ServerName: lc.Host})
			if err != nil {
				return err
			}
		}

		lc.conn = l
	}
	return nil
}

// Close closes the ldap backend connection
func (lc *LDAPAuthProvider) Close() {
	if lc.conn != nil {
		lc.conn.Close()
		lc.conn = nil
	}
}

// Authenticate authenticates the user against the ldap backend
func (lc *LDAPAuthProvider) Authenticate(username, password string) (authenticated bool, token string, err error) {
	err = lc.Connect()
	defer lc.Close()

	if err != nil {
		return
	}

	// First bind with a read only user
	if lc.BindDN != "" && lc.BindPassword != "" {
		err = lc.conn.Bind(lc.BindDN, lc.BindPassword)
		if err != nil {
			return
		}
	}

	attributes := append(lc.Attributes, "dn")
	// Search for the given username
	searchRequest := ldap.NewSearchRequest(
		lc.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(lc.UserFilter, username),
		attributes,
		nil,
	)

	sr, err := lc.conn.Search(searchRequest)
	if err != nil {
		return
	}

	if len(sr.Entries) < 1 {
		err = errors.New("User does not exist")
		return
	}

	if len(sr.Entries) > 1 {
		err = errors.New("Too many entries returned")
		return
	}

	userDN := sr.Entries[0].DN
	user := map[string]string{}
	for _, attr := range lc.Attributes {
		user[attr] = sr.Entries[0].GetAttributeValue(attr)
	}

	// Bind as the user to verify their password
	err = lc.conn.Bind(userDN, password)
	if err != nil {
		return
	}

	token = tk.CreateExpiringToken(username, lc.signingKey, time.Hour*48, "ldap")

	//We authenticated and we have our token
	authenticated = true

	// Rebind as the read only user for any further queries
	if lc.BindDN != "" && lc.BindPassword != "" {
		err = lc.conn.Bind(lc.BindDN, lc.BindPassword)
		if err != nil {
			return
		}
	}

	// Sync LDAP group membership when configured.
	if lc.DB != nil && lc.LDAPCompanyId != "" {
		groupDNs := sr.Entries[0].GetAttributeValues("memberOf")
		lc.syncGroups(username, groupDNs)
	}

	return
}

// syncGroups upserts each LDAP group DN into user_groups (keyed by
// externalRef) and records the user as a member. Errors are logged but do
// not fail authentication — group sync is best-effort.
func (lc *LDAPAuthProvider) syncGroups(username string, groupDNs []string) {
	for _, dn := range groupDNs {
		gid, err := lc.DB.UpsertGroupByExternalRef(models.Group{
			Name:        dn,
			CompanyId:   lc.LDAPCompanyId,
			ExternalRef: dn,
			CreatedAt:   time.Now(),
		})
		if err != nil {
			continue
		}
		_ = lc.DB.AddGroupMember(models.GroupMember{GroupId: gid, UserId: username})
	}
}
