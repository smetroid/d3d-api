// Package membership provides audience-resolution helpers for element shares.
// It is intentionally kept separate from the postgres package so handlers can
// depend on a narrow interface and tests can use a fake store.
package membership

import "github.com/smetroid/d3d-api/app/models"

// DB is the subset of postgres.Postgres required for audience resolution.
type DB interface {
	GetUserCompanyIds(username string) ([]string, error)
	GetUserGroupIds(username string) ([]string, error)
}

// UserInAudience reports whether username is permitted by audience.
//
//   - "public"  — always true (no DB lookup)
//   - "user"    — true if username appears in audience.Ids
//   - "company" — true if the user is a member of any company in audience.Ids
//   - "group"   — true if the user is a member of any group in audience.Ids
func UserInAudience(db DB, username string, audience models.AudienceSpec) (bool, error) {
	switch audience.Kind {
	case "public":
		return true, nil

	case "user":
		for _, uid := range audience.Ids {
			if uid == username {
				return true, nil
			}
		}
		return false, nil

	case "company":
		companyIds, err := db.GetUserCompanyIds(username)
		if err != nil {
			return false, err
		}
		idSet := toSet(companyIds)
		for _, aid := range audience.Ids {
			if idSet[aid] {
				return true, nil
			}
		}
		return false, nil

	case "group":
		groupIds, err := db.GetUserGroupIds(username)
		if err != nil {
			return false, err
		}
		idSet := toSet(groupIds)
		for _, aid := range audience.Ids {
			if idSet[aid] {
				return true, nil
			}
		}
		return false, nil
	}

	return false, nil
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
